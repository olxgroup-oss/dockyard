package aws

import (
	"errors"
	"fmt"

	"regexp"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
)

func getAsg(
	asgName string,
	session *session.Session,
) (*autoscaling.Group, error) {

	autoScalingCl := autoscaling.New(session)

	input := &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{
			aws.String(asgName),
		},
	}
	result, err := autoScalingCl.DescribeAutoScalingGroups(input)
	if err != nil {
		return nil, err
	}
	return result.AutoScalingGroups[0], err
}

// Fetches all Auto Scaling groups in the region
func (asgRollout *asgRolloutClient) FetchAsg() ([][]string, error) {

	asgInfos, err := asgRollout.FetchAsgWithTags([]string{})

	if err != nil {
		return [][]string{}, err
	} else {
		return asgRollout.FormatAsgs(asgInfos), nil
	}
}

// Fetches Auto Scaling groups of EKS cluster (eg. example-cluster) having tags as
// kubernetes.io/cluster/example-cluster : owner
func (asgRollout *asgRolloutClient) FetchAsgOfEks(
	eksClusterName string,
) ([][]string, error) {
	log.Debug("Fetching all asgs registered with cluster", eksClusterName)
	asgInfos, err := asgRollout.FetchAsgWithTags(
		[]string{fmt.Sprintf("tag:kubernetes.io/cluster/%v", eksClusterName)},
	)

	if err != nil {
		return [][]string{}, err
	} else {
		return asgRollout.FormatAsgs(asgInfos), nil
	}
}

func (asgRollout *asgRolloutClient) FetchAsgWithTags(
	tags []string,
) ([]AsgInfo, error) {
	autoScalingSvc := autoscaling.New(asgRollout.session)

	filters := []*autoscaling.Filter{}

	for _, tag := range tags {
		name := tag
		value := "owned"

		filters = append(
			filters,
			&autoscaling.Filter{Name: &name, Values: []*string{&value}},
		)
	}

	input := &autoscaling.DescribeAutoScalingGroupsInput{
		Filters: filters,
	}

	result, err := autoScalingSvc.DescribeAutoScalingGroups(input)

	if err != nil {
		return nil, err
	}

	asgInfos := []AsgInfo{}
	asgToInstanceIdsMap := map[string][]*string{}
	allInstanceIds := []*string{}

	ec2Svc := ec2.New(asgRollout.session)

	amiIds := []*string{}

	for _, group := range result.AutoScalingGroups {

		var defaultAmiId *string

		oldInstances, newInstances, defaultAmiId, err := asgRollout.getOldnNewInstancesOfAsg(
			group,
		)

		if err != nil {
			return nil, err
		}

		log.Debug(
			"For ASG ",
			*group.AutoScalingGroupName,
			" oldInstances: ",
			len(oldInstances),
			" newInstances: ",
			len(newInstances),
		)

		instanceIds := append(oldInstances, newInstances...)

		allInstanceIds = append(allInstanceIds, instanceIds...)

		asgToInstanceIdsMap[*group.AutoScalingGroupName] = instanceIds

		var asgProgress *int
		if len(instanceIds) > 0 {
			progressPercentage := (len(newInstances) * 100) / len(instanceIds)
			asgProgress = &progressPercentage
		} else {
			progressPercentage := 0
			asgProgress = &progressPercentage
		}

		asgInfo := AsgInfo{
			Name:             *group.AutoScalingGroupName,
			DesiredInstances: *group.DesiredCapacity,
			Progress:         asgProgress,
			MinInstances:     *group.MinSize,
			MaxInstances:     *group.MaxSize,
			InstanceIds:      instanceIds,
		}

		if defaultAmiId != nil {
			amiIds = append(amiIds, defaultAmiId)
			asgInfo.AmiId = *defaultAmiId
		}

		asgInfos = append(asgInfos, asgInfo)
	}

	describeImagesInput := &ec2.DescribeImagesInput{
		ImageIds: amiIds,
	}

	imageResult, err := ec2Svc.DescribeImages(describeImagesInput)

	if err != nil {
		return nil, err
	}

	amiNames := map[string]string{}

	for _, image := range imageResult.Images {
		amiNames[*image.ImageId] = *image.Name
	}

	newAsgInfos := []AsgInfo{}

	for _, asgInfo := range asgInfos {
		asgInfo.Ami = amiNames[asgInfo.AmiId]
		newAsgInfos = append(newAsgInfos, asgInfo)
	}

	return newAsgInfos, nil
}

// Separate out old and new instances for this asg
func (asgRollout *asgRolloutClient) GetOldnNewInstancesOfAsg(
	asgName string,
) (oldInstances []*string, newInstances []*string, err error) {
	autoScalingSvc := autoscaling.New(asgRollout.session)

	input := &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []*string{
			aws.String(asgName),
		},
	}

	result, err := autoScalingSvc.DescribeAutoScalingGroups(input)

	if err != nil {
		return []*string{}, []*string{}, err
	}

	if len(result.AutoScalingGroups) == 0 {
		return []*string{}, []*string{}, fmt.Errorf(
			"no autoscaling group found with name %v",
			asgName,
		)
	}

	oldInstances, newInstances, _, err = asgRollout.getOldnNewInstancesOfAsg(
		result.AutoScalingGroups[0],
	)

	return
}

// Returns an array of same length as instanceIds. If result[i] has
// value true, instanceIds[i] is a new instance otherwise old.
func (asgRollout *asgRolloutClient) AreInstancesNew(
	asgName string, instanceIds []string,
) (result []bool, err error) {
	_, newInstances, err := asgRollout.GetOldnNewInstancesOfAsg(asgName)

	if err != nil {
		return
	}

	for _, instanceId := range instanceIds {
		result = append(
			result,
			StringPointerSliceContains(newInstances, &instanceId),
		)
	}

	return
}

func (asgRollout *asgRolloutClient) getOldnNewInstancesOfAsg(
	group *autoscaling.Group,
) (oldInstances []*string, newInstances []*string, defaultAmiId *string, err error) {
	autoScalingSvc := autoscaling.New(asgRollout.session)
	ec2Svc := ec2.New(asgRollout.session)

	var launchTemplateInput *ec2.DescribeLaunchTemplateVersionsInput

	latestVersionNumber := -1
	latestLaunchTemplateId := ""
	launchConfigName := ""

	isLaunchConfig := false

	log.Debug("Determine old and new instances based on LT, LC or MixedInstancesPolicy for ", *group.AutoScalingGroupName)
	if group.LaunchConfigurationName != nil {
		log.Debug("Launch Coniguration found ", *group.AutoScalingGroupName)
		isLaunchConfig = true
		launchConfigName = *group.LaunchConfigurationName

		launchTemplateInput = &ec2.DescribeLaunchTemplateVersionsInput{
			LaunchTemplateId: group.LaunchConfigurationName,
		}

		describeLaunchConfigInput := &autoscaling.DescribeLaunchConfigurationsInput{
			LaunchConfigurationNames: []*string{
				group.LaunchConfigurationName,
			},
		}

		result, err := autoScalingSvc.DescribeLaunchConfigurations(
			describeLaunchConfigInput,
		)

		if err != nil {
			return []*string{}, []*string{}, nil, err
		}

		if len(result.LaunchConfigurations) > 0 {
			defaultAmiId = result.LaunchConfigurations[0].ImageId
		}

	} else if group.LaunchTemplate != nil {
		log.Debug("Launch Template found ", *group.AutoScalingGroupName)
		launchTemplateInput = &ec2.DescribeLaunchTemplateVersionsInput{
			LaunchTemplateId: group.LaunchTemplate.LaunchTemplateId,
			// Versions:         aws.StringSlice([]string{"$Latest"}),
		}

		launchTemplatesResult, err := ec2Svc.DescribeLaunchTemplateVersions(launchTemplateInput)

		if err != nil {
			return []*string{}, []*string{}, nil, err
		}

		for _, launchTemplate := range launchTemplatesResult.LaunchTemplateVersions {

			if *launchTemplate.VersionNumber > int64(latestVersionNumber) {
				latestVersionNumber = int(*launchTemplate.VersionNumber)
				latestLaunchTemplateId = *launchTemplate.LaunchTemplateId
			}
		}

		if len(launchTemplatesResult.LaunchTemplateVersions) > 0 {
			defaultAmiId = launchTemplatesResult.LaunchTemplateVersions[0].LaunchTemplateData.ImageId
		}
	} else if group.MixedInstancesPolicy != nil {
		log.Debug("MixedInstancesPolicy found ", *group.AutoScalingGroupName)
		launchTemplateInput = &ec2.DescribeLaunchTemplateVersionsInput{
			LaunchTemplateId: group.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification.LaunchTemplateId,
		}

		launchTemplatesResult, err := ec2Svc.DescribeLaunchTemplateVersions(launchTemplateInput)

		if err != nil {
			return []*string{}, []*string{}, nil, err
		}

		for _, launchTemplate := range launchTemplatesResult.LaunchTemplateVersions {

			if *launchTemplate.VersionNumber > int64(latestVersionNumber) {
				latestVersionNumber = int(*launchTemplate.VersionNumber)
				latestLaunchTemplateId = *launchTemplate.LaunchTemplateId
			}

		}

		if len(launchTemplatesResult.LaunchTemplateVersions) > 0 {
			defaultAmiId = launchTemplatesResult.LaunchTemplateVersions[0].LaunchTemplateData.ImageId
		}

	}

	instanceIds := []*string{}

	for _, instance := range group.Instances {
		instanceIds = append(instanceIds, instance.InstanceId)
	}

	oldInstances = []*string{}
	newInstances = []*string{}

	if !isLaunchConfig {
		for _, instance := range group.Instances {

			if instance.LaunchTemplate != nil {
				currentVersion, err := strconv.Atoi(
					*instance.LaunchTemplate.Version,
				)

				if err == nil {
					if latestLaunchTemplateId == *instance.LaunchTemplate.LaunchTemplateId &&
						currentVersion == latestVersionNumber {
						newInstances = append(newInstances, instance.InstanceId)
					} else {
						oldInstances = append(oldInstances, instance.InstanceId)
					}
				} else {
					oldInstances = append(oldInstances, instance.InstanceId)
				}
			} else {
				oldInstances = append(oldInstances, instance.InstanceId)
			}
		}
	} else {
		for _, instance := range group.Instances {

			if instance.LaunchConfigurationName != nil {

				if *instance.LaunchConfigurationName == launchConfigName {
					newInstances = append(newInstances, instance.InstanceId)
				} else {
					oldInstances = append(oldInstances, instance.InstanceId)
				}
			} else {
				oldInstances = append(oldInstances, instance.InstanceId)
			}
		}
	}
	return
}

// Returns ec2 image details of the provided ami ids
func (asgRollout *asgRolloutClient) GetAmiDetails(
	imageIds []*string,
) ([]*ec2.Image, error) {
	ec2Svc := ec2.New(asgRollout.session)

	describeImagesInput := &ec2.DescribeImagesInput{
		ImageIds: imageIds,
	}

	imageResult, err := ec2Svc.DescribeImages(describeImagesInput)

	if err != nil {
		return nil, err
	}

	return imageResult.Images, nil
}

func (asgRollout *asgRolloutClient) FormatAsgs(asgs []AsgInfo) [][]string {
	result := [][]string{
		{"#", "ASG Name", "Desired", "AMI", "Progress", "Min/Max"},
	}

	for i, asg := range asgs {
		minMax := fmt.Sprintf("%v/%v", asg.MinInstances, asg.MaxInstances)

		progress := ""

		if asg.Progress == nil {
			progress = "NA"
		} else {
			progress = fmt.Sprintf("%v%%", *asg.Progress)
		}
		// fmt.Println(aws.Progress)
		result = append(
			result,
			[]string{
				fmt.Sprint(i),
				asg.Name,
				fmt.Sprint(asg.DesiredInstances),
				string(asg.Ami),
				progress,
				minMax,
			},
		)
	}

	return result
}

// Returns eks version from the ami name
func (asgRollout *asgRolloutClient) GetEksVersionFromAmiName(
	amiName string,
) (string, error) {
	re := regexp.MustCompile(`eks-\d+.\d+|node-\d+.\d+`)
	reVersions := regexp.MustCompile(`\d+.\d+`)

	matches := re.FindAllString(amiName, -1)

	if len(matches) > 0 {

		versionMatches := reVersions.FindAllString(matches[0], -1)

		if len(versionMatches) > 0 {
			return versionMatches[0], nil
		} else {
			return "", errors.New(fmt.Sprint("Couldn't parse version from ami name. Provied: ", amiName))
		}
	} else {
		return "", errors.New(fmt.Sprint("Couldn't parse version from ami name. Provied: ", amiName))
	}
}

// Returns instance ids of all instances for this asg
func (asgRollout *asgRolloutClient) GetInstancesOfAsg(
	asgName string,
) ([]*string, error) {
	asg, err := getAsg(asgName, asgRollout.session)
	if err != nil {
		return nil, err
	}
	instanceList := make([]*string, 0)
	for _, instance := range asg.Instances {
		instanceList = append(instanceList, instance.InstanceId)
	}
	return instanceList, nil
}

// Returns ec2 instance array of the asg
func (asgRollout *asgRolloutClient) GetInstanceDetailsOfAsg(
	asgName string,
) ([]*ec2.Instance, error) {
	instances, err := asgRollout.GetInstancesOfAsg(asgName)

	if err != nil {
		return nil, err
	}

	ec2Cl := ec2.New(asgRollout.session)
	ec2input := &ec2.DescribeInstancesInput{
		InstanceIds: instances,
	}

	nodesResult, err := ec2Cl.DescribeInstances(ec2input)

	if err != nil {
		return nil, err
	}

	instanceArray := []*ec2.Instance{}

	for _, reservation := range nodesResult.Reservations {
		instanceArray = append(instanceArray, reservation.Instances...)
	}

	return instanceArray, nil

}

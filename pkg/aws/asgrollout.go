package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
)

var (
	progress = &RolloutProgress{
		StepsSize: int32(0),
		StepsDone: int32(0),
		TotalSize: int32(0),
	}
)

type AsgRolloutConfig struct {
	IgnoreNotFound  bool           `mapstructure:"IGNORE_NOT_FOUND"`
	PeriodWait      rolloutPeriod  `mapstructure:"PERIOD_WAIT"`
	Timeout         rolloutTimeout `mapstructure:"TIMEOUTS"`
	PrivateRegistry string         `mapstructure:"PRIVATE_REGISTRY"`
	ForceDeletePods bool           `mapstructure:"FORCE_DELETE_PODS"`
	EksClusterName  string         `mapstructure:"EKS_CLUSTER_NAME"`
}

type rolloutPeriod struct {
	// time to wait before executing post rollout steps
	BeforePost int64 `mapstructure:"BEFORE_POST"`
	// time to wait after a batch is processed
	AfterBatch int64 `mapstructure:"AFTER_BATCH"`
	// time to wait for k8sNode to be ready
	WaitForReady int64 `mapstructure:"K8S_READY"`
	// time to wait for new ec2 instance to join asg
	WaitForNewNode int64 `mapstructure:"NEW_NODE_ASG_REGISTER"`
}

type rolloutTimeout struct {
	NewNodeTimeout int64 `mapstructure:"NEW_NODE_ASG_REGISTER"`
}

// Struct to denote a progress of rollout
// at a specific time.
type RolloutProgress struct {
	StepsSize int32
	StepsDone int32
	TotalSize int32
}

// Channel to let the receiver know the current
// progress of rollout. The rolllout goroutine would
// continuously send progress info to this channel
type RolloutProgressChan chan RolloutProgress

type AsgRolloutClient interface {

	// FetchAsg Fetches all Auto Scaling groups in the region
	FetchAsg() ([][]string, error)

	// FetchAsgOfEks Fetches Auto Scaling groups of EKS cluster (eg. example-cluster) having tags as
	// kubernetes.io/cluster/example-cluster : owner
	FetchAsgOfEks(string) ([][]string, error)

	// Returns desired capacity of the provided asg
	GetDesiredCount(asgName string) (int64, error)

	// Sets new desired capacity of capacity for the asgName asg
	SetDesiredCount(asgName string, capacity int64) error

	// Returns min size of the asg
	GetMinCount(asgName string) (int64, error)

	// Sets min size of the asg
	SetMinCount(asgName string, capacity int64) error

	// Returns max size of the asg
	GetMaxCount(asgName string) (int64, error)

	// Sets max count of the asg
	SetMaxCount(asgName string, capacity int64) error

	// Enables scale in instance protection for this asg.
	EnableNewInstanceProtection(asgName string) error

	// Disables scale in instance projection for this asg
	DisableNewInstanceProtection(asgName string) error

	// Adds a given tag key and value to an asg
	AddTagToAsG(asgName, tagKey, tagValue string) error

	// Returns instance ids of all instances for this asg
	GetInstancesOfAsg(asgName string) ([]*string, error)

	// Returns ec2 instance array of the asg
	GetInstanceDetailsOfAsg(asgName string) ([]*ec2.Instance, error)

	// Returns ec2 image details of the provided ami ids
	GetAmiDetails(imageIds []*string) ([]*ec2.Image, error)

	// Returns Private DNS Name of the provided instanceId
	GetNodeNameFromInstanceId(instanceId string) (*string, error)

	// Returns health status of the asg
	GetAsgHealth(asgName string) (bool, error)

	// Separate out old and new instances for this asg
	GetOldnNewInstancesOfAsg(
		string,
	) (oldInstances []*string, newInstances []*string, err error)

	// Returns instance id from the instance's Private DNS Name
	GetInstanceIdFromNodeName(nodeName, asgName string) (string, error)

	// Returns eks version from the ami name
	GetEksVersionFromAmiName(amiName string) (string, error)

	// Returns an array of same length as instanceIds. If result[i] has
	// value true, instanceIds[i] is a new instance otherwise old.
	AreInstancesNew(
		asgName string, instanceIds []string,
	) (result []bool, err error)

	// Returns value of tag tagKey for this asg
	GetTagValueOfAsg(asgName, tagKey string) (int64, error)

	// Deletes tag with key tagKey of this asg
	DeleteTagOfAsg(asgName, tagKey, tagVal string) error

	// Checks if dockyard asg upgrade has already been started
	UpgradeStarted(asgName string) (bool, error)

	// Checks if dockyard rollout has completed
	RolloutCompleted(asgName string) (bool, error)

	// Perform prerollout steps of the asg such as add node-state, min
	// and max tags on asg etc
	PreRolloutStart(asgName string, eventLogs chan string, rolloutProgressChan RolloutProgressChan) error

	//Returns array of new instance ids for this asg
	GetNewNodes(asgName string) ([]string, error)

	// Fetches all old nodes for this asg and returns ids of
	// batchSize nodes
	NodesToDrain(asgName string, batchSize int) ([]string, error)

	// Perform post rolloout steps like clean up tags, restoring min
	// and max of the asg
	PostRolloutStart(
		asgName string,
		rolloutProgressChan RolloutProgressChan,
		eventLogs chan string,
		rolloutSuccess bool,
	) error

	// Starts dockyard rollout
	StartRollout(
		ctx context.Context,
		asgName string,
		batchSize int64,
		rolloutProgressChan RolloutProgressChan,
		eventLogs chan string,
	) error

	// Terminate instance with private DNS name nodeName
	// of this asg
	TerminateInstance(nodeName, asgName string) error

	// Returns healthy status of the instance
	IsInstanceHealthy(instanceId string) (bool, error)

	// Disables instance scale in protection for this particular
	// instance in asg
	RemoveInstanceScaleInProtection(instanceId, asgName string) error
}

// Fetches all Auto Scaling groups in the region
func StringSliceContains(slice []string, item string) bool {
	for _, val := range slice {
		if val == item {
			return true
		}
	}

	return false
}

func StringPointerSliceContains(slice []*string, item *string) bool {
	if item == nil {
		return false
	}

	stringSlice := []string{}

	for _, str := range slice {
		if str != nil {
			stringSlice = append(stringSlice, *str)
		}
	}

	return StringSliceContains(stringSlice, *item)
}

// Enables scale in instance protection for this asg.
func (asgRollout *asgRolloutClient) EnableNewInstanceProtection(
	asgName string,
) error {
	svc := autoscaling.New(asgRollout.session)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName:             aws.String(asgName),
		NewInstancesProtectedFromScaleIn: aws.Bool(true),
	}
	_, err := svc.UpdateAutoScalingGroup(input)
	return err
}

// Disables scale in instance projection for this asg
func (asgRollout *asgRolloutClient) DisableNewInstanceProtection(
	asgName string,
) error {

	svc := autoscaling.New(asgRollout.session)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName:             aws.String(asgName),
		NewInstancesProtectedFromScaleIn: aws.Bool(false),
	}
	_, err := svc.UpdateAutoScalingGroup(input)
	return err
}

// Returns health status of the asg
func (asgRollout *asgRolloutClient) GetAsgHealth(asgName string) (bool, error) {
	asg, err := getAsg(asgName, asgRollout.session)
	if err != nil {
		return false, err
	}
	for _, instance := range asg.Instances {
		if *instance.HealthStatus != "Healthy" {
			return false, nil
		}
	}
	return true, nil
}

// Checks if dockyard asg upgrade has already been started
func (asgRollout *asgRolloutClient) UpgradeStarted(
	asgName string,
) (bool, error) {
	old, err := asgRollout.kube.GetNodeCountByLabel(
		getNodeStateLabel("old"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)
	if err != nil {
		return false, fmt.Errorf(
			"Unable to fetch nodes with label %s",
			fmt.Sprintf("%v,err ", getNodeStateLabel("old")),
		)
	}
	new, err := asgRollout.kube.GetNodeCountByLabel(
		getNodeStateLabel("new"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)
	if err != nil {
		return false, fmt.Errorf(
			"Unable to fetch nodes with label %s",
			fmt.Sprintf("%v,err ", getNodeStateLabel("new")),
		)
	}
	if old == 0 && new == 0 {
		return false, nil
	} else {
		return true, nil
	}
}

// Checks if dockyard rollout has completed
func (asgRollout *asgRolloutClient) RolloutCompleted(
	asgName string,
) (bool, error) {
	oldNodes, err := asgRollout.kube.GetNodeCountByLabel(
		getNodeStateLabel("old"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)
	if err != nil {
		return false, err
	}
	if oldNodes > 0 {
		return false, nil
	} else {
		return true, nil
	}
}

// Perform prerollout steps of the asg such as add node-state labels, min
// and max tags on asg etc
func (asgRollout *asgRolloutClient) PreRolloutStart(
	asgName string,
	eventLogs chan string,
	rolloutProgressChan RolloutProgressChan,
) error {

	eventLogs <- "Starting prerollout execution"
	log.Infof("Started prerollout execution for asg %s", asgName)
	instances, newInstances, err := asgRollout.GetOldnNewInstancesOfAsg(asgName)
	if err != nil {
		log.Errorf("unable to fetch instance details for asg %s", asgName)
		return fmt.Errorf("Unable to fetch Instances of asg %s", asgName)
	}

	// labelling new instances
	for _, instance := range newInstances {
		k8sNode, err := asgRollout.GetNodeNameFromInstanceId(*instance)
		if err != nil {
			log.Errorf("unable to fetch k8sNode for instance %s due to %s", *instance, err.Error())
			return fmt.Errorf(
				"Unable to get k8s Node for instance with id %s",
				*instance,
			)
		}
		if len(*k8sNode) == 0 {
			continue
		}
		eventLogs <- fmt.Sprintf("Ignoring node %s for rollout", *k8sNode)
		log.Infof("Ignoring node %s for rollout ", *k8sNode)
		err = asgRollout.kube.AddLabelToNode(
			*k8sNode,
			NodeStateLabelKey,
			"new",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		if err != nil {
			log.Errorf("Unable to label k8s Node %s due to ", *k8sNode, err.Error())
			return fmt.Errorf("Unable to label k8s Node %s", err.Error())
		}
	}

	// labelling old instances
	for _, instance := range instances {
		k8sNode, err := asgRollout.GetNodeNameFromInstanceId(*instance)
		if err != nil {
			log.Errorf("unable to fetch k8sNode for instance %s due to %s", *instance, err.Error())
			return fmt.Errorf(
				"Unable to get k8s Node for instance with id %s",
				*instance,
			)
		}
		if len(*k8sNode) == 0 {
			continue
		}
		eventLogs <- fmt.Sprintf("Marking node %s for rollout", *k8sNode)
		log.Infof("Marking node %s for rollout ", *k8sNode)
		err = asgRollout.kube.AddLabelToNode(
			*k8sNode,
			NodeStateLabelKey,
			"old",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		if err != nil {
			log.Errorf("Unable to label k8s Node %s due to %s", *k8sNode, err.Error())
			return fmt.Errorf("Unable to label k8s Node %s", err.Error())
		}
	}

	asgMax, err := asgRollout.GetMaxCount(asgName)
	if err != nil {
		log.Errorf("Unable to fetch max count for asg %s due to %s", asgName, err.Error())
		return err
	}
	asgMin, err := asgRollout.GetMinCount(asgName)
	if err != nil {
		log.Errorf("Unable to fetch min count for asg %s due to %s", asgName, err.Error())
		return err
	}
	asgDesired, err := asgRollout.GetDesiredCount(asgName)
	if err != nil {
		log.Errorf("Unable to fetch desired count for asg %s due to %s", asgName, err.Error())
		return err
	}

	desiredNodes, _ := asgRollout.GetTagValueOfAsg(
		asgName,
		"dockyard.io/desired",
	)

	//	First Time Pre rollout execution since tags are already stored
	// No need to store tags again
	if desiredNodes == 0 {
		eventLogs <- "Storing intial asg state in asg tags"
		err = asgRollout.AddTagToAsG(
			asgName,
			"dockyard.io/min",
			strconv.Itoa(int(asgMin)),
		)
		if err != nil {
			log.Errorf("Unable tag asg %s due to %s", asgName, err.Error())
			return err
		}
		log.Infof("Asg %s tagged with key=dockyard.io/min value=%s ", asgName, asgMin)
		err = asgRollout.AddTagToAsG(
			asgName,
			"dockyard.io/max",
			strconv.Itoa(int(asgMax)),
		)
		if err != nil {
			log.Errorf("Unable tag asg %s due to %s", asgName, err.Error())
			return err
		}
		log.Infof("Asg %s tagged with key=dockyard.io/max value=%s", asgName, asgMax)
		err = asgRollout.AddTagToAsG(
			asgName,
			"dockyard.io/desired",
			strconv.Itoa(int(asgDesired)),
		)
		if err != nil {
			log.Errorf("Unable tag asg %s due to %s", asgName, err.Error())
			return err
		}
		log.Infof("Asg %s tagged with key=dockyard.io/desired value=%s", asgName, asgDesired)
	}

	currentAsgNodes := make([]string, 0)
	inst, err := asgRollout.GetInstancesOfAsg(asgName)
	if err != nil {
		return err
	}

	for _, i := range inst {
		kNode, err := asgRollout.GetNodeNameFromInstanceId(*i)

		if err != nil {
			log.Errorf("Unable k8sNode from instanceid %s due to %s", *i, err.Error())
			return fmt.Errorf(
				"Unable to get k8s Node for instance with id %s",
				*kNode,
			)
		}
		currentAsgNodes = append(currentAsgNodes, *kNode)
	}

	for _, k8sNode := range currentAsgNodes {
		if len(k8sNode) == 0 {
			continue
		}
		// Handle error
		eventLogs <- fmt.Sprintf("Cordon node %s", k8sNode)
		log.Infof("Cordon node %s", k8sNode)
		err = asgRollout.kube.CordonNode(k8sNode, asgRollout.rolloutConfig.IgnoreNotFound)
		if err != nil {
			log.Errorf("Unable to cordon Node %s due to %s ", k8sNode, err.Error())
			return fmt.Errorf("Unable to cordon Node %s,%s ", k8sNode, err)
		}
	}

	eventLogs <- fmt.Sprintf("Enabling new instance protection for asg %s", asgName)
	err = asgRollout.EnableNewInstanceProtection(asgName)
	if err != nil {
		log.Errorf("Unable to set new Instance Protection for asg %s due to %s", asgName, err.Error())
		return fmt.Errorf(
			"Unable to set new Instance Protection %s",
			err.Error(),
		)
	}
	log.Infof("Enabling new instance protection for asg %s", asgName)

	//}

	progress.StepsDone = 1
	rolloutProgressChan <- *progress

	eventLogs <- "Pre rollout steps executed"
	log.Infof("Pre rollout steps executed for asg %s", asgName)
	return nil
}

// Fetches all old nodes for this asg and returns ids of
// batchSize nodes
func (asgRollout *asgRolloutClient) NodesToDrain(
	asgName string,
	batchSize int,
) ([]string, error) {
	nodeList := make([]string, 0)
	nodes, err := asgRollout.kube.GetNodeByLabel(
		getNodeStateLabel("old"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)

	if err != nil {
		return []string{}, nil
	}
	count := 0
	for _, node := range nodes {
		count += 1
		nodeList = append(nodeList, node.Name)
		if count == batchSize {
			return nodeList, nil
		}
	}
	return []string{}, nil
}

// Perform post rolloout steps like clean up tags, restoring min
// and max of the asg
func (asgRollout *asgRolloutClient) PostRolloutStart(
	asgName string,
	rolloutProgressChan RolloutProgressChan,
	eventLogs chan string,
	rolloutSuccess bool,
) error {

	eventLogs <- fmt.Sprintf("Starting post rollout execution")
	log.Infof("Starting post rollout execution for asg %s", asgName)
	nodes, err := asgRollout.kube.GetNodeByLabel(
		getNodeStateLabel("new"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)
	if err != nil {
		log.Errorf("Unable to fetch k8sNode by label due to %s", err.Error())
		return err
	}
	for _, node := range nodes {

		if len(node.Name) == 0 {
			continue
		}
		eventLogs <- fmt.Sprintf("Removing labels of node %s", node.Name)
		asgRollout.kube.RemoveLabel(
			node.Name,
			NodeStateLabelKey,
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		log.Infof("Removing label %s for node %s", NodeStateLabelKey, node.Name)
	}

	nodes, err = asgRollout.kube.GetNodeByLabel(
		getNodeStateLabel("old"),
		asgRollout.rolloutConfig.IgnoreNotFound,
	)
	if err != nil {
		log.Errorf("Unable to fetch k8sNode by label due to %s", err.Error())
		return err
	}
	for _, node := range nodes {

		if len(node.Name) == 0 {
			continue
		}
		eventLogs <- fmt.Sprintf("Removing labels of node %s", node.Name)
		// Handle error
		asgRollout.kube.RemoveLabel(
			node.Name,
			NodeStateLabelKey,
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		log.Infof("Removing label %s for node %s ", NodeStateLabelKey, node.Name)
	}

	currentAsgNodes := make([]string, 0)
	inst, err := asgRollout.GetInstancesOfAsg(asgName)
	if err != nil {
		log.Errorf("Unable to fetch instance of asg %s due to %s", asgName, err.Error())
		return err
	}

	for _, i := range inst {
		kNode, _ := asgRollout.GetNodeNameFromInstanceId(*i)
		currentAsgNodes = append(currentAsgNodes, *kNode)
	}

	for _, node := range currentAsgNodes {
		if len(node) == 0 {
			continue
		}
		// Handle error
		eventLogs <- fmt.Sprintf("Uncordon node  %s", node)
		log.Infof("Uncordon node %s", node)
		asgRollout.kube.UnCordonNode(node, asgRollout.rolloutConfig.IgnoreNotFound)
	}

	minNodes, err := asgRollout.GetTagValueOfAsg(asgName, "dockyard.io/min")

	if err != nil {
		log.Errorf("Unable to fetch tags of asg %s due to %s", asgName, err.Error())
		return err
	}
	maxNodes, err := asgRollout.GetTagValueOfAsg(asgName, "dockyard.io/max")

	if err != nil {
		log.Errorf("Unable to fetch tags of asg %s due to %s", asgName, err.Error())
		return err
	}
	desiredNodes, err := asgRollout.GetTagValueOfAsg(
		asgName,
		"dockyard.io/desired",
	)

	if err != nil {
		log.Errorf("Unable to fetch tags of asg %s due to %s", asgName, err.Error())
		return err
	}

	err = asgRollout.DeleteTagOfAsg(
		asgName,
		"dockyard.io/min",
		strconv.Itoa(int(minNodes)),
	)

	if err != nil {
		return err
	}

	eventLogs <- fmt.Sprintf("Deleting tag %s of asg %s", "dockyard.io/max", asgName)
	log.Infof("Deleting tag %s of asg %s ", "dockyard.io/max", asgName)
	err = asgRollout.DeleteTagOfAsg(
		asgName,
		"dockyard.io/max",
		strconv.Itoa(int(maxNodes)),
	)

	if err != nil {
		log.Errorf("Unable to delete tags of asg %s due to %s", asgName, err.Error())
		return err
	}

	eventLogs <- fmt.Sprintf("Deleting tag %s of asg %s", "dockyard.io/desired", asgName)
	log.Infof("Deleting tag %s of asg %s", "dockyard.io/desired", asgName)
	err = asgRollout.DeleteTagOfAsg(
		asgName,
		"dockyard.io/desired",
		strconv.Itoa(int(desiredNodes)),
	)

	if err != nil {
		log.Errorf("Unable to delete tags of asg %s due to %s", asgName, err.Error())
		return err
	}
	log.Infof("Updating desired count %s for asgName %s ", desiredNodes, asgName)
	err = asgRollout.SetMinCount(asgName, minNodes)

	eventLogs <- fmt.Sprintf("Updating min count of asg %s to previous state ", asgName)
	if err != nil {
		log.Errorf("Unable to update tags of asg %s due to %s", asgName, err.Error())
		return err
	}
	log.Infof("Updating min count %s for asgName %s ", minNodes, asgName)
	err = asgRollout.SetMaxCount(asgName, maxNodes)
	eventLogs <- fmt.Sprintf("Updating max count of asg %s to previous state ", asgName)
	if err != nil {
		log.Errorf("Unable to update tags of asg %s due to %s", asgName, err.Error())
		return err
	}

	log.Infof("Updating max count %s for asgName %s ", maxNodes, asgName)
	time.Sleep(1 * time.Minute)
	instances, _ := asgRollout.GetInstancesOfAsg(asgName)

	for _, instance := range instances {
		eventLogs <- fmt.Sprintf("Removing instance scale in protection for instance %s ", *instance)
		// handle error
		asgRollout.RemoveInstanceScaleInProtection(*instance, asgName)
		log.Infof("Removing instance scale in protection for instance %s,%s", *instance, asgName)
	}

	if !rolloutSuccess {
		eventLogs <- fmt.Sprintf("Disabling new Instance Protection for asg %s", asgName)
		err := asgRollout.DisableNewInstanceProtection(asgName)
		if err != nil {
			log.Errorf("Unable to unset instance protection of asg %s due to %s", asgName, err.Error())
			eventLogs <- fmt.Sprintf("Unable to unset new Instance Protection %s", err.Error())
		}
		log.Infof("Removed instance scale in protection for asg %s", asgName)
		eventLogs <- fmt.Sprintf("Post rollout steps executed")
		log.Infof("Post rollout steps executed for asg %s", asgName)
		return nil
	}
	// Stuck issue if post excuted before successful rollout
	progress.StepsDone = 1
	rolloutProgressChan <- *progress

	eventLogs <- fmt.Sprintf("Post rollout steps executed")
	//close(eventLogs)
	return nil
}

// Starts dockyard rollout
// TODO: Implement Semaphore
// TODO: Parse Context to all goroutines
func (asgRollout *asgRolloutClient) StartRollout(
	ctx context.Context,
	asgName string,
	batchSize int64,
	rolloutProgressChan RolloutProgressChan,
	eventLogs chan string,
) error {
	oldInstances, _, err := asgRollout.GetOldnNewInstancesOfAsg(asgName)
	if err != nil {
		return fmt.Errorf("Unable to fetch Instances of asg %s", asgName)
	}
	countOldInstances := len(oldInstances)

	// +2 is for executing preRollout, postRollout
	progress.TotalSize = int32(countOldInstances + 2)
	progress.StepsSize = int32(batchSize)
	rolloutProgressChan <- *progress

	err = asgRollout.PreRolloutStart(asgName, eventLogs, rolloutProgressChan)
	if err != nil {
		log.Errorf("Unable to execute pre rollout stage for asg %s due to %s", asgName, err.Error())
		return fmt.Errorf(
			"Unable to execute pre rollout steps, %v",
			err.Error(),
		)
	}

	steps := int(float64(countOldInstances / int(batchSize)))
	log.Infof("Number of iterations for entire rollout %d", steps)
	var w sync.WaitGroup

	asgMax, err := asgRollout.GetMaxCount(asgName)
	if err != nil {
		log.Errorf("Unable to fetch max count for asg %s due to %s", asgName, err.Error())
		return err
	}
	log.Infof("Max count for asg %s is %d ", asgName, asgMax)
	asgDesired, err := asgRollout.GetDesiredCount(asgName)
	if err != nil {
		log.Errorf("Unable to fetch desired for asg %s due to %s", asgName, err.Error())
		return err
	}

	log.Infof("Desired count for asg %s is %d ", asgName, asgDesired)
	if asgDesired+batchSize > asgMax {
		asgRollout.SetMaxCount(asgName, asgMax+batchSize)
		eventLogs <- fmt.Sprintf("Updating max count of asg to %v", asgMax+batchSize)
		log.Infof("Updating max count for asg %s is %d ", asgName, asgMax+batchSize)
	}
	log.Infof("Updating min count for asg %s is %d ", asgName, asgDesired+batchSize)
	asgRollout.SetMinCount(asgName, asgDesired+batchSize)
	eventLogs <- fmt.Sprintf("Updating min count of asg to %v", asgDesired+batchSize)
	log.Infof("Updating desired count for asg %s is %d ", asgName, asgDesired+batchSize)
	asgRollout.SetDesiredCount(asgName, asgDesired+batchSize)
	eventLogs <- fmt.Sprintf("Updating desired count of asg to %v", asgDesired+batchSize)
	lastBatch := false
	for i := 0; i < steps; i++ {

		// all instances joining after the last batch should not have instance protection enabled
		if i == steps-1 {
			lastBatch = true
		}

		errChan := make(chan error)
		errors := make([]error, 0)
		nodes, err := asgRollout.NodesToDrain(asgName, int(batchSize))
		if err != nil {
			log.Errorf("Unable to fetch nodes of asg %s for draining due to %s", asgName, err.Error())
			return err
		}
		// There are no nodes left to drain
		if len(nodes) == 0 {
			break
		}

		for node := 0; node < int(batchSize); node++ {
			w.Add(1)

			log.Infof("Rollout started for node %s ", nodes[node])
			eventLogs <- fmt.Sprintf("Rollout started for node %s", nodes[node])
			go rolloutNode(
				ctx,
				asgRollout,
				asgName,
				nodes[node],
				errChan,
				&w,
				lastBatch,
				eventLogs,
			)
		}

		//block till all old nodes in a single batch are recycled
		go func(w *sync.WaitGroup, errChan chan error) {
			w.Wait()
			defer close(errChan)
		}(&w, errChan)

		////block till all olds nodes from batch is recycled
		for err := range errChan {
			progress.StepsDone = 1
			rolloutProgressChan <- *progress
			if err != nil {
				errors = append(errors, err)

			}
		}

		//return if any of the nodes within the batch is not recycled
		if len(errors) > 0 {
			errString := make([]string, 0)
			for _, e := range errors {
				errString = append(errString, e.Error())
			}
			log.Errorf("Unable to rollout nodes due to %s", err.Error())
			return fmt.Errorf(
				"Unable to rollout nodes %v",
				strings.Join(errString, ","),
			)
		}

		time.Sleep(time.Duration(asgRollout.rolloutConfig.PeriodWait.AfterBatch) * time.Second)
	}
	return nil
	//return asgRollout.PostRolloutStart(asgName)
}

func rolloutNode(
	ctx context.Context,
	asgRollout *asgRolloutClient,
	asgName, nodeName string,
	errCh chan error,
	w *sync.WaitGroup,
	lastBatch bool,
	eventLogs chan string,
) {

	defer w.Done()

	// TODO handle if new node not found retry logic

	// TODO should propogate error channel here
	var newNode string
	nodeFound := make(chan string, 1)
	errC := make(chan error)

	timeout := time.Duration(asgRollout.rolloutConfig.Timeout.NewNodeTimeout)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)

	go asgRollout.getNewNode(ctxWithTimeout, asgName, nodeFound, errC, eventLogs)

	log.Infof("Waiting to provision new nodes for asg %s ", asgName)
	// blocking till we have an error or a new node or a timeout

	select {
	case e := <-errC:
		if e != nil {
			errCh <- e
			log.Errorf("Unable to provision new nodes for asg %s ", asgName)
			cancel()
			return
		} else {
			newNode = <-nodeFound
			//errCh <- nil
		}
	case <-ctx.Done():

		switch ctx.Err() {

		case context.DeadlineExceeded:
			errCh <- fmt.Errorf("unable to get new node, Timeout Exceeded")
			log.Errorf("Unable to provision new nodes for asg %s, DeadlineExceeded ", asgName)

		case context.Canceled:
			errCh <- fmt.Errorf("unable to get new node")
		}

	}
	cancel()
	isReady := make(chan error, 1)

	eventLogs <- fmt.Sprintf("Waiting for new k8s node to be in Ready state")
	log.Infof("Waiting to provision new nodes for asg %s to be in healthy state", asgName)
	// Parse context with timeout
	go func(isReady chan error, ctx context.Context) {
		for {
			isHealthy, err := asgRollout.kube.IsNodeHealthy(
				newNode,
				asgRollout.rolloutConfig.IgnoreNotFound,
			)
			if err != nil {
				isReady <- err
				log.Errorf("Unable to fetch health status of the node")
				return
			}
			if isHealthy {
				isReady <- nil
				return
			} else {
				time.Sleep(time.Duration(asgRollout.rolloutConfig.PeriodWait.WaitForReady) * time.Second)
			}
		}
	}(isReady, ctx)

	// NewNode is healthy

	if isReadyError := <-isReady; isReadyError == nil {

		eventLogs <- fmt.Sprintf("New Node %s is in Ready state", newNode)
		log.Infof("New node %s registered with the cluster", newNode)
		err := asgRollout.kube.AddLabelToNode(
			newNode,
			NodeStateLabelKey,
			"new",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		if err != nil {
			errCh <- err
			return
		}
		eventLogs <- fmt.Sprintf("Started draining node %s", nodeName)
		log.Infof("Started drainng node %s", nodeName)
		errs := asgRollout.kube.DrainNode(
			ctx,
			nodeName,
			true,
			asgRollout.rolloutConfig.ForceDeletePods,
			true,
			asgRollout.rolloutConfig.IgnoreNotFound,
			eventLogs,
		)

		if len(errs) != 0 {
			errCh <- fmt.Errorf("Unable to drain node %s, %s", nodeName, errs)
			return
		}

		eventLogs <- fmt.Sprintf("Node %s drained successfully", nodeName)
		log.Infof("Node %s drained successfully", nodeName)
		err = asgRollout.kube.AddLabelToNode(
			nodeName,
			NodeStateLabelKey,
			"drained",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		if err != nil {
			errCh <- err
			return
		}
		// No error in all pods eviction
		eventLogs <- fmt.Sprintf("Deleting Node %s ", nodeName)
		log.Infof("Deleting node %s", nodeName)
		err = asgRollout.kube.DeleteNode(nodeName, asgRollout.rolloutConfig.IgnoreNotFound)

		if err != nil {
			errC <- err
			return
		}
		if lastBatch {
			err := asgRollout.DisableNewInstanceProtection(asgName)
			if err != nil {
				log.Errorf("Unable to unset new Instance Protection for asg %s due to %s", asgName, err.Error())
				errCh <- fmt.Errorf("Unable to unset new Instance Protection %s", err.Error())
			}
		}
		log.Infof("Terminating instance %s of asg %s", nodeName, asgName)
		errCh <- asgRollout.TerminateInstance(nodeName, asgName)
		eventLogs <- fmt.Sprintf("Terminating Instance %s ", nodeName)
	} else {
		errCh <- isReadyError
		return
	}

}

// Terminate instance
func (asgRollout *asgRolloutClient) TerminateInstance(
	nodeName, asgName string,
) error {
	instanceId, err := asgRollout.GetInstanceIdFromNodeName(nodeName, asgName)
	if err != nil {
		return err
	}
	ec2Cl := ec2.New(asgRollout.session)
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceId),
		},
	}

	_, err = ec2Cl.TerminateInstances(input)
	return err
}

func getNodeStateLabel(state string) string {
	return fmt.Sprintf("%v=%v", NodeStateLabelKey, state)
}

// Disables instance scale in protection
func (asgRollout *asgRolloutClient) RemoveInstanceScaleInProtection(
	instanceId, asgName string,
) error {
	svc := autoscaling.New(asgRollout.session)
	input := &autoscaling.SetInstanceProtectionInput{
		AutoScalingGroupName: aws.String(asgName),
		InstanceIds: []*string{
			aws.String(instanceId),
		},
		ProtectedFromScaleIn: aws.Bool(false),
	}

	_, err := svc.SetInstanceProtection(input)
	return err
}

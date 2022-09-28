package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Returns Private DNS Name of the provided instanceId
func (asgRollout *asgRolloutClient) GetNodeNameFromInstanceId(
	instanceId string,
) (*string, error) {
	ec2Cl := ec2.New(asgRollout.session)
	ec2input := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceId}),
	}

	nodesResult, err := ec2Cl.DescribeInstances(ec2input)

	if err != nil {
		return nil, err
	}

	//var nt *string
	for _, reservation := range nodesResult.Reservations {

		if len(reservation.Instances) != 0 {
			return reservation.Instances[0].PrivateDnsName, nil

			//tags := reservation.Instances[0].Tags
			//for _, t := range tags {
			//	if *t.Key == "Name" {
			//		nt = t.Value
			//		break
			//	}
			//}
			//return nt, nil
		}
	}

	return nil, errors.New("no node name found in any reservations")
}

// Returns instance id from the instance's Private DNS Name
func (asgRollout *asgRolloutClient) GetInstanceIdFromNodeName(
	nodeName, asgName string,
) (string, error) {

	instances, err := asgRollout.GetInstanceDetailsOfAsg(asgName)
	if err != nil {
		return "", err
	}

	for _, instance := range instances {

		if *instance.PrivateDnsName == nodeName {
			return *instance.InstanceId, nil
		}

	}

	return "", fmt.Errorf("Unable to fetch instanceId of node %s", nodeName)
}

//Returns array of new instance ids for this asg
func (asgRollout *asgRolloutClient) GetNewNodes(
	asgName string,
) ([]string, error) {
	instances, err := asgRollout.GetInstancesOfAsg(asgName)
	if err != nil {
		return []string{}, err
	}
	newNodeList := make([]string, 0)
	for _, instance := range instances {
		k8sNode, err := asgRollout.GetNodeNameFromInstanceId(*instance)
		if err != nil {
			return []string{}, err
		}
		hasLabel, err := asgRollout.kube.NodeHasLabel(
			*k8sNode,
			NodeStateLabelKey,
			"old",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		hasLabel2, err := asgRollout.kube.NodeHasLabel(
			*k8sNode,
			NodeStateLabelKey,
			"new",
			asgRollout.rolloutConfig.IgnoreNotFound,
		)
		if err != nil {
			return []string{}, err
		}
		if !hasLabel && !hasLabel2 {
			newNodeList = append(newNodeList, *k8sNode)
		}
	}
	return newNodeList, nil
}

func (asgRollout *asgRolloutClient) getNewNode(
	ctx context.Context,
	asgName string,
	node chan string,
	errChan chan error,
	eventLogs chan string,
) {

	eventLogs <- fmt.Sprintf("Waiting for new node to join ASG %s", asgName)
	found := false
	for {

		instances, err := asgRollout.GetInstancesOfAsg(asgName)
		if err != nil {
			errChan <- err
			break
		}
		for _, instance := range instances {

			ec2Healhty, err := asgRollout.IsInstanceHealthy(*instance)

			if err != nil {
				errChan <- err
				break
			}
			// Will skip this instance since it's not yet in ready state
			if !ec2Healhty {
				continue
			}
			k8sNode, err := asgRollout.GetNodeNameFromInstanceId(*instance)
			if err != nil {
				errChan <- err
				break
			}
			hasLabel, err := asgRollout.kube.NodeHasLabel(
				*k8sNode,
				NodeStateLabelKey,
				"old",
				asgRollout.rolloutConfig.IgnoreNotFound,
			)
			if apierrors.IsNotFound(err) {
				continue
			}

			if err != nil {
				errChan <- err
				break
			}
			hasLabel2, err := asgRollout.kube.NodeHasLabel(
				*k8sNode,
				NodeStateLabelKey,
				"new",
				asgRollout.rolloutConfig.IgnoreNotFound,
			)
			if err != nil {
				errChan <- err
				break
			}
			if !hasLabel && !hasLabel2 {
				found = true
				node <- *k8sNode
				errChan <- nil
				break
			}
		}

		if !found {
			time.Sleep(time.Duration(asgRollout.rolloutConfig.PeriodWait.WaitForNewNode) * time.Second)
		} else {
			break
		}

	}

	eventLogs <- fmt.Sprintf("New node has joined ASG %s", asgName)

}

// Returns healthy status of the instance
func (asgRollout *asgRolloutClient) IsInstanceHealthy(
	instanceId string,
) (bool, error) {
	svc := ec2.New(asgRollout.session)
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds: []*string{
			aws.String(instanceId),
		},
	}

	result, err := svc.DescribeInstanceStatus(input)
	if err != nil {
		return false, err
	}

	for _, status := range result.InstanceStatuses {
		if *status.SystemStatus.Status == "ok" {
			return true, nil
		} else {

			return false, nil
		}
	}
	return false, nil
}

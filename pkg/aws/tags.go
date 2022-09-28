package aws

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
)

var (
	NodeStateLabelKey = "dockyard.io/node-state"
)

// Returns desired capacity of the provided asg
func (asgRollout *asgRolloutClient) GetDesiredCount(
	asgName string,
) (int64, error) {
	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	asg, err := getAsg(asgName, asgRollout.session)
	if err != nil {
		return 0, err
	} else {
		return *asg.DesiredCapacity, nil
	}

}

// Sets new desired capacity of capacity for the asgName asg
func (asgRollout *asgRolloutClient) SetDesiredCount(
	asgName string,
	capacity int64,
) error {

	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	autoscaligCl := autoscaling.New(asgRollout.session)
	input := &autoscaling.SetDesiredCapacityInput{
		AutoScalingGroupName: aws.String(asgName),
		DesiredCapacity:      aws.Int64(capacity),
		HonorCooldown:        aws.Bool(true),
	}

	_, err := autoscaligCl.SetDesiredCapacity(input)
	return err
}

// Returns min size of the asg
func (asgRollout *asgRolloutClient) GetMinCount(asgName string) (int64, error) {
	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	asg, err := getAsg(asgName, asgRollout.session)
	if err != nil {
		return 0, err
	} else {
		return *asg.MinSize, nil
	}
}

// Sets min size of the asg
func (asgRollout *asgRolloutClient) SetMinCount(
	asgName string,
	capacity int64,
) error {
	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	autoscaligCl := autoscaling.New(asgRollout.session)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(asgName),
		MinSize:              aws.Int64(capacity),
	}

	_, err := autoscaligCl.UpdateAutoScalingGroup(input)
	return err
}

// Returns max size of the asg
func (asgRollout *asgRolloutClient) GetMaxCount(asgName string) (int64, error) {

	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	asg, err := getAsg(asgName, asgRollout.session)
	if err != nil {
		return 0, err
	} else {
		return *asg.MaxSize, nil
	}
}

// Sets max count of the asg
func (asgRollout *asgRolloutClient) SetMaxCount(
	asgName string,
	capacity int64,
) error {

	asgRollout.lock.Lock()
	defer asgRollout.lock.Unlock()
	autoscaligCl := autoscaling.New(asgRollout.session)
	input := &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(asgName),
		MaxSize:              aws.Int64(capacity),
	}

	_, err := autoscaligCl.UpdateAutoScalingGroup(input)
	return err
}

// Adds a given tag key and value to an asg
func (asgRollout *asgRolloutClient) AddTagToAsG(
	asgName, tagKey, tagValue string,
) error {
	autoscaligCl := autoscaling.New(asgRollout.session)
	_, err := autoscaligCl.CreateOrUpdateTags(
		&autoscaling.CreateOrUpdateTagsInput{
			Tags: []*autoscaling.Tag{
				{
					PropagateAtLaunch: aws.Bool(false),
					ResourceId:        aws.String(asgName),
					ResourceType:      aws.String("auto-scaling-group"),
					Key:               aws.String(tagKey),
					Value:             aws.String(tagValue),
				},
			},
		},
	)
	return err
}

// Returns value of tag tagKey for this asg
func (asgRollout *asgRolloutClient) GetTagValueOfAsg(
	asgName, tagKey string,
) (int64, error) {

	svc := autoscaling.New(asgRollout.session)
	input := &autoscaling.DescribeTagsInput{
		Filters: []*autoscaling.Filter{
			{
				Name: aws.String("auto-scaling-group"),
				Values: []*string{
					aws.String(asgName),
				},
			},
		},
	}

	result, err := svc.DescribeTags(input)

	if err != nil {
		return 0, err
	}
	for _, tag := range result.Tags {
		if *tag.Key == tagKey {
			v, _ := strconv.Atoi(*tag.Value)
			return int64(v), nil
		}
	}
	return 0, fmt.Errorf(
		"Tag with key %s for asg %s not found",
		asgName,
		tagKey,
	)
}

// Deletes tag with key tagKey of this asg
func (asgRollout *asgRolloutClient) DeleteTagOfAsg(
	asgName, tagKey, tagVal string,
) error {

	autoscaligCl := autoscaling.New(asgRollout.session)
	_, err := autoscaligCl.DeleteTags(&autoscaling.DeleteTagsInput{
		Tags: []*autoscaling.Tag{
			{
				ResourceId:   aws.String(asgName),
				ResourceType: aws.String("auto-scaling-group"),
				Key:          aws.String(tagKey),
				Value:        aws.String(tagVal),
			},
		},
	})
	return err
}

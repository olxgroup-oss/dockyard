package aws

import (
	"context"
	//"dockyard/config"

	"dockyard/pkg/kube"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"os"
	"sync"
)

type InstanceInfo struct {
	InstanceId string   `json:"instance_id"`
	ImageId    string   `json:"image_id"`
	Version    *float64 `json:"version"`
}

type AsgInfo struct {
	Name             string    `json:"name"`
	DesiredInstances int64     `json:"desired_instances"`
	Ami              string    `json:"ami"`
	AmiId            string    `json:"ami_id"`
	Progress         *int      `json:"progress"`
	MinInstances     int64     `json:"min_instances"`
	MaxInstances     int64     `json:"max_instances"`
	InstanceIds      []*string `json:"instance_ids"`
}

type asgRolloutClient struct {
	session       *session.Session
	kube          kube.KubeClient
	lock          sync.Mutex
	rolloutConfig *AsgRolloutConfig
}

func NewAsgRollout(ctx context.Context, config *AwsConfig, client kube.KubeClient, rolloutConfig *AsgRolloutConfig) AsgRolloutClient {

	if config != nil {
		os.Setenv("AWS_REGION", config.Region)
		os.Setenv("AWS_PROFILE", config.Profile)
	}

	sess, err := session.NewSessionWithOptions(
		session.Options{SharedConfigState: session.SharedConfigEnable},
	)

	if err != nil {
		log.Fatal(err)
	}

	return &asgRolloutClient{
		session:       sess,
		kube:          client,
		lock:          sync.Mutex{},
		rolloutConfig: rolloutConfig,
	}
}

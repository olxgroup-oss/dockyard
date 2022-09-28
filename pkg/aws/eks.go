package aws

import (
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
	"github.com/aws/aws-sdk-go/service/servicequotas"
	"github.com/aws/aws-sdk-go/service/servicequotas/servicequotasiface"
)

const (
	// EC2OnDemandServiceQuotaCode is running On-Demand Standard (A, C, D, H, I, M, R, T, Z) instances
	EC2OnDemandServiceQuotaCode = "L-1216C47A"
	// EC2SpotServiceQuotaCode is all Standard (A, C, D, H, I, M, R, T, Z) Spot Instance Requests
	EC2SpotServiceQuotaCode = "L-34B43A08"
)

type AwsEksClient interface {
	AvailableIp() ([][]string, error)
	Ec2Limits() ([][]string, error)
}

type awsEksClient struct {
	session     *session.Session
	clusterName string
}

// Fix profile parsing
func NewAwsEKS(clusterName, profile string) AwsEksClient {

	sess, err := session.NewSessionWithOptions(
		session.Options{
			Profile:           profile,
			SharedConfigState: session.SharedConfigEnable,
		},
	)

	if err != nil {
		log.Fatal(err)
	}

	return &awsEksClient{
		session:     sess,
		clusterName: clusterName,
	}
}

// AvailableIp returns available Ips in each subnet that is registered with eks cluster

func (eksClient *awsEksClient) AvailableIp() ([][]string, error) {
	var eksCl eksiface.EKSAPI = eks.New(eksClient.session)
	var ec2Cl ec2iface.EC2API = ec2.New(eksClient.session)
	cluster, err := eksCl.DescribeCluster(
		&eks.DescribeClusterInput{Name: &eksClient.clusterName},
	)
	if err != nil {
		return nil, err
	}

	vpcId := cluster.Cluster.ResourcesVpcConfig.VpcId
	filterName := "vpc-id"
	subnets, err := ec2Cl.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   &filterName,
				Values: []*string{vpcId},
			},
		},
	})

	if err != nil {
		return nil, err
	}
	res := make([][]string, 0)
	for _, subnet := range subnets.Subnets {
		res = append(
			res,
			[]string{
				*subnet.SubnetId,
				*subnet.AvailabilityZone,
				strconv.Itoa(int(*subnet.AvailableIpAddressCount)),
			},
		)
	}
	return res, nil
}

// Returns quota limits for ondemand and spot instances
func (eksClient *awsEksClient) Ec2Limits() ([][]string, error) {
	var serviceQuotasCl servicequotasiface.ServiceQuotasAPI = servicequotas.New(eksClient.session)
	ec2ServiceCode := "ec2"
	quotas, err := serviceQuotasCl.ListServiceQuotas(
		&servicequotas.ListServiceQuotasInput{ServiceCode: &ec2ServiceCode},
	)
	res := make([][]string, 0)
	if err != nil {
		return res, err
	}
	for _, quota := range quotas.Quotas {
		if *quota.QuotaCode == EC2SpotServiceQuotaCode {
			res = append(
				res,
				[]string{"Spot Limit", strconv.Itoa(int(*quota.Value))},
			)
		}
		if *quota.QuotaCode == EC2OnDemandServiceQuotaCode {
			res = append(
				res,
				[]string{"OnDemand Limit", strconv.Itoa(int(*quota.Value))},
			)
		}
	}
	return res, nil
}

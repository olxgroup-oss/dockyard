package aws

type AwsConfig struct {
	Region  string `mapstructure:"AWS_REGION"  validate:"required"`
	Profile string `mapstructure:"AWS_PROFILE" validate:"required"`
}

func NewAwsConfig(region, profile string) AwsConfig {
	return AwsConfig{
		Region:  region,
		Profile: profile,
	}
}

func (a AwsConfig) GetProfile() string {
	return a.Profile
}

func (a AwsConfig) GetRegion() string {
	return a.Region
}

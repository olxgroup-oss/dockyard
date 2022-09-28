package config

import (
	"dockyard/pkg/aws"
	"dockyard/utils"

	"github.com/go-playground/validator"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	AwsConfig  *aws.AwsConfig        `mapstructure:"AWS_CONFIG"`
	Logging    *utils.LoggingConfig  `mapstructure:"LOGGING"`
	AsgRollout *aws.AsgRolloutConfig `mapstructure:"ASG_ROLLOUT"`
}

// Reads config.yaml from current working directory and sets configuration for dockyard
func LoadConfig() (config Config, err error) {
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Setting Defaults
	viper.SetDefault(
		"ASG_ROLLOUT",
		map[string]interface{}{
			"IGNORE_NOT_FOUND":  true,
			"FORCE_DELETE_PODS": false,
			"PERIOD_WAIT": map[string]interface{}{
				"BEFORE_POST":           60,
				"AFTER_BATCH":           30,
				"K8S_READY":             30,
				"NEW_NODE_ASG_REGISTER": 30,
			},
			"TIMEOUTS": map[string]interface{}{
				"NEW_NODE_ASG_REGISTER": 600,
			},
		},
	)
	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)

	if err != nil {
		return
	}

	validate := validator.New()

	err = validate.Struct(&config)

	if err != nil {
		return
	}

	if config.Logging != nil && config.Logging.Level != nil {
		_, err = log.ParseLevel(*config.Logging.Level)
	}

	return
}

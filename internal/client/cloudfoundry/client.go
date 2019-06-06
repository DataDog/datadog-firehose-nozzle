package cloudfoundry

import (
	"fmt"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
)

func New(config *config.Config, logger *gosteno.Logger) (*cfclient.Client, error) {
	if config.CloudControllerEndpoint == "" {
		logger.Warnf("the Cloud Controller Endpoint needs to be set in order to set up the cf client")
		return nil, fmt.Errorf("the Cloud Controller Endpoint needs to be set in order to set up the cf client")
	}

	cfg := cfclient.Config{
		ApiAddress:        config.CloudControllerEndpoint,
		ClientID:          config.Client,
		ClientSecret:      config.ClientSecret,
		SkipSslValidation: config.InsecureSSLSkipVerify,
		UserAgent:         "datadog-firehose-nozzle",
	}
	cfClient, err := cfclient.NewClient(&cfg)
	if err != nil {
		logger.Warnf("encountered an error while setting up the cf client: %v", err)
		return nil, fmt.Errorf("encountered an error while setting up the cf client: %v", err)
	}

	return cfClient, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/cloudfoundry/gosteno"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

const (
	authorizationHeaderKey = "Authorization"
	// RealIPHeader refers to the cluster level check runner ip passed in the request headers
	RealIPHeader = "X-Real-Ip"
)

// DCAClient is required to query the API of Datadog cluster agent
type DCAClient struct {
	clusterAgentAPIEndpoint       string  // ${SCHEME}://${clusterAgentHost}:${PORT}
	ClusterAgentVersion           Version // Version of the cluster-agent we're connected to
	clusterAgentAPIClient         *http.Client
	clusterAgentAPIRequestHeaders http.Header
	logger                        *gosteno.Logger
}

func NewDCAClient(config *config.Config, logger *gosteno.Logger) (*DCAClient, error) {
	var err error

	dcaClient := DCAClient{}
	dcaClient.logger = logger
	dcaClient.clusterAgentAPIEndpoint = config.DCAUrl
	authToken := config.DCAToken
	if authToken == "" {
		return nil, fmt.Errorf("missing authentication token for the Cluster Agent Client")
	}

	dcaClient.clusterAgentAPIRequestHeaders = http.Header{}
	dcaClient.clusterAgentAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))
	dcaClient.clusterAgentAPIClient = util.GetClient(false)
	dcaClient.clusterAgentAPIClient.Timeout = 2 * time.Second

	// Validate the cluster-agent client by checking the version
	dcaClient.ClusterAgentVersion, err = dcaClient.GetVersion()
	if err != nil {
		return nil, err
	}

	logger.Infof("Successfully connected to the Datadog Cluster Agent %s", dcaClient.ClusterAgentVersion.String())

	return &dcaClient, nil
}

// Version returns ClusterAgentVersion already stored in the DCAClient
func (c *DCAClient) Version() Version {
	return c.ClusterAgentVersion
}

// GetVersion fetches the version of the Cluster Agent. Used in the agent status command.
func (c *DCAClient) GetVersion() (Version, error) {
	const dcaVersionPath = "version"
	var version Version
	var err error

	// https://host:port/version
	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaVersionPath)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return version, err
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	resp, err := c.clusterAgentAPIClient.Do(req)
	if err != nil {
		return version, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return version, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return version, err
	}

	err = json.Unmarshal(body, &version)

	return version, err
}

// TODO
func (c *DCAClient) GetApplications() ([]CFApplication, error) {
	const dcaAppsPath = "api/v1/cf/apps"
	var cfapps []CFApplication
	var err error

	// https://host:port/api/v1/cf/apps
	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaAppsPath)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	resp, err := c.clusterAgentAPIClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &cfapps)

	return cfapps, err
}

// TODO
func (c *DCAClient) GetApplication(appGUID string) (*CFApplication, error) {
	const dcaAppsPath = "api/v1/cf/apps"
	var cfapp CFApplication
	var err error

	// https://host:port/api/v1/cf/apps/{appGUID}
	rawURL := fmt.Sprintf("%s/%s/%s", c.clusterAgentAPIEndpoint, dcaAppsPath, appGUID)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	resp, err := c.clusterAgentAPIClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &cfapp)

	return &cfapp, err
}
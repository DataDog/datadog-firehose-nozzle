// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"strings"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/retry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/cloudfoundry-community/go-cfclient"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

const (
	authorizationHeaderKey = "Authorization"
	// RealIPHeader refers to the cluster level check runner ip passed in the request headers
	RealIPHeader = "X-Real-Ip"
)

var globalClusterAgentClient *DCAClient

// DCAClientInterface  is required to query the API of Datadog cluster agent
type DCAClientInterface interface {
	Version() Version
	ClusterAgentAPIEndpoint() string

	GetVersion() (Version, error)
	GetCFAppsMetadataForNode(nodename string) (map[string][]string, error)
	GetCFApps() ([]cfclient.V3App, error)
}

// DCAClient is required to query the API of Datadog cluster agent
type DCAClient struct {
	// used to setup the DCAClient
	initRetry retry.Retrier

	clusterAgentAPIEndpoint       string  // ${SCHEME}://${clusterAgentHost}:${PORT}
	ClusterAgentVersion           Version // Version of the cluster-agent we're connected to
	clusterAgentAPIClient         *http.Client
	clusterAgentAPIRequestHeaders http.Header
}

// resetGlobalClusterAgentClient is a helper to remove the current DCAClient global
// It is ONLY to be used for tests
func resetGlobalClusterAgentClient() {
	globalClusterAgentClient = nil
}

// GetClusterAgentClient returns or init the DCAClient
func GetClusterAgentClient() (DCAClientInterface, error) {
	if globalClusterAgentClient == nil {
		globalClusterAgentClient = &DCAClient{}
		globalClusterAgentClient.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "clusterAgentClient",
			AttemptMethod:     globalClusterAgentClient.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	if err := globalClusterAgentClient.initRetry.TriggerRetry(); err != nil {
		fmt.Printf("Cluster Agent init error: %v", err)
		return nil, err
	}
	return globalClusterAgentClient, nil
}

func (c *DCAClient) init() error {
	var err error

	c.clusterAgentAPIEndpoint, err = getClusterAgentEndpoint()
	if err != nil {
		return err
	}

	authToken := config.NozzleConfig.DCAToken
	if authToken != "" {
		return fmt.Errorf("missing authentication token for the Cluster Agent Client")
	}

	c.clusterAgentAPIRequestHeaders = http.Header{}
	c.clusterAgentAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))

	// TODO remove insecure
	c.clusterAgentAPIClient = util.GetClient(false)
	c.clusterAgentAPIClient.Timeout = 2 * time.Second

	// Validate the cluster-agent client by checking the version
	c.ClusterAgentVersion, err = c.GetVersion()
	if err != nil {
		return err
	}
	fmt.Printf("Successfully connected to the Datadog Cluster Agent %s", c.ClusterAgentVersion.String())
	return nil
}

// Version returns ClusterAgentVersion already stored in the DCAClient
func (c *DCAClient) Version() Version {
	return c.ClusterAgentVersion
}

// ClusterAgentAPIEndpoint returns the Agent API Endpoint URL as a string
func (c *DCAClient) ClusterAgentAPIEndpoint() string {
	return c.clusterAgentAPIEndpoint
}

// getClusterAgentEndpoint provides a validated https endpoint from configuration keys in datadog.yaml:
// configuration key "cluster_agent.url" (or the DD_CLUSTER_AGENT_URL environment variable),
//      add the https prefix if the scheme isn't specified
func getClusterAgentEndpoint() (string, error) {
	dcaURL := config.NozzleConfig.DCAUrl
	if dcaURL != "" {
		if strings.HasPrefix(dcaURL, "http://") {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", dcaURL)
		}
		if strings.Contains(dcaURL, "://") == false {
			fmt.Printf("Adding https scheme to %s: https://%s", dcaURL, dcaURL)
			dcaURL = fmt.Sprintf("https://%s", dcaURL)
		}
		u, err := url.Parse(dcaURL)
		if err != nil {
			return "", err
		}
		if u.Scheme != "https" {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", u.Scheme)
		}
		fmt.Printf("Connecting to the configured URL for the Datadog Cluster Agent: %s", dcaURL)
		return u.String(), nil
	}

	return "", fmt.Errorf("Cluster Agent URL is not specified in the configuration")
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

// GetCFAppsMetadataForNode returns the CF application tags from the Cluster Agent.
func (c *DCAClient) GetCFAppsMetadataForNode(nodename string) (map[string][]string, error) {
	const dcaCFAppsMeta = "api/v1/tags/cf/apps"
	var err error
	var tags map[string][]string

	// https://host:port/api/v1/tags/cf/apps/{nodename}
	rawURL := fmt.Sprintf("%s/%s/%s", c.clusterAgentAPIEndpoint, dcaCFAppsMeta, nodename)

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
	err = json.Unmarshal(body, &tags)
	return tags, err
}

// TODO
func (c *DCAClient) GetCFApps() ([]cfclient.V3App, error) {
	const dcaCFApps = "api/v1/cf/apps"
	var err error
	var apps []*cfclient.V3App

	// https://host:port/api/v1/cf/apps
	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaCFApps)

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
	err = json.Unmarshal(body, &apps)
	return apps, err
}

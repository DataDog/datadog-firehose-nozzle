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
	GetCFApps() ([]CFApplication, error)
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
	if authToken == "" {
		return fmt.Errorf("missing authentication token for the Cluster Agent Client")
	}

	c.clusterAgentAPIRequestHeaders = http.Header{}
	c.clusterAgentAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))

	// TODO remove insecure
	c.clusterAgentAPIClient = util.GetClient(false)
	c.clusterAgentAPIClient.Timeout = 5 * time.Second

	// Validate the cluster-agent client by checking the version
	c.ClusterAgentVersion, err = c.GetVersion()
	if err != nil {
		return err
	}
	fmt.Printf("Successfully connected to the Datadog Cluster Agent %s", c.ClusterAgentVersion.String())

	apps, err := c.GetCFApps()
	if err != nil {
		fmt.Printf("ERROR GETTING CF Apps from DCA Client AFTER INIT METHOD: %v/n", err)
	}

	for i, app := range apps {
		out, err := json.MarshalIndent(app, "", "    ")
		if err != nil {
			fmt.Printf("error marshalling DCA app: %v", err)
		}
		fmt.Printf("App %d: %v", i, string(out))
	}
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
func (c *DCAClient) GetCFApps() ([]CFApplication, error) {
	const dcaCFApps = "api/v1/cf/apps"
	var err error
	var apps []CFApplication

	// https://host:port/api/v1/cf/apps/
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
	var v3Apps []cfclient.V3App
	err = json.Unmarshal(body, &v3Apps)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling V3App response: %d", err)
	}

	for _, v3App := range v3Apps {
		cfapp := CFApplication{}
		cfapp.setV3AppData(v3App)
		apps = append(apps, cfapp)

	}
	return apps, err
}

// // TODO
// func (c *DCAClient) GetCFSpaces() ([]cfclient.V3Space, error) {
// 	const dcaCFSpaces = "api/v1/cf/spaces/"
// 	var err error
// 	var spaces []cfclient.V3Space

// 	// https://host:port/api/v1/cf/spaces
// 	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaCFSpaces)

// 	req, err := http.NewRequest("GET", rawURL, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header = c.clusterAgentAPIRequestHeaders

// 	resp, err := c.clusterAgentAPIClient.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
// 	}

// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, err
// 	}
// 	err = json.Unmarshal(body, &spaces)
// 	return spaces, err
// }

// // TODO
// func (c *DCAClient) GetCFOrgs() ([]cfclient.V3Organization, error) {
// 	const dcaCFOrgs = "api/v1/cf/orgs/"
// 	var err error
// 	var orgs []cfclient.V3Organization

// 	// https://host:port/api/v1/cf/orgs
// 	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaCFOrgs)

// 	req, err := http.NewRequest("GET", rawURL, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header = c.clusterAgentAPIRequestHeaders

// 	resp, err := c.clusterAgentAPIClient.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
// 	}

// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, err
// 	}
// 	err = json.Unmarshal(body, &orgs)
// 	return orgs, err
// }

// // TODO
// func (c *DCAClient) GetCFProcesses() ([]cfclient.Process, error) {
// 	const dcaCFProcesses = "api/v1/cf/processes/"
// 	var err error
// 	var processes []cfclient.Process

// 	// https://host:port/api/v1/cf/processes
// 	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaCFProcesses)

// 	req, err := http.NewRequest("GET", rawURL, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header = c.clusterAgentAPIRequestHeaders

// 	resp, err := c.clusterAgentAPIClient.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
// 	}

// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, err
// 	}
// 	err = json.Unmarshal(body, &processes)
// 	return processes, err
// }

// func (c *DCAClient) getV3Applications() ([]CFApplication, error) {
// 	var wg sync.WaitGroup
// 	errors := make(chan error, 10)
// 	// Fetch apps
// 	wg.Add(1)
// 	var cfapps []cfclient.V3App
// 	go func() {
// 		defer wg.Done()
// 		var err error
// 		cfapps, err = c.GetCFApps()
// 		if err != nil {
// 			errors <- err
// 		}
// 	}()

// 	// Fetch processes
// 	wg.Add(1)
// 	var processes []cfclient.Process
// 	go func() {
// 		defer wg.Done()
// 		var err error
// 		processes, err = c.GetCFProcesses()
// 		if err != nil {
// 			errors <- err
// 		}
// 	}()

// 	// Fetch spaces
// 	wg.Add(1)
// 	var spaces []cfclient.V3Space
// 	go func() {
// 		defer wg.Done()
// 		var err error
// 		spaces, err = c.GetCFSpaces()
// 		if err != nil {
// 			errors <- err
// 		}
// 	}()

// 	// Fetch orgs
// 	wg.Add(1)
// 	var orgs []cfclient.V3Organization
// 	go func() {
// 		defer wg.Done()
// 		var err error
// 		orgs, err = c.GetCFOrgs()
// 		if err != nil {
// 			errors <- err
// 		}
// 	}()

// 	wg.Wait()
// 	close(errors)

// 	// Go through the channel, print all errors and return one of them if there are any
// 	var err error
// 	for err = range errors {
// 		c.logger.Error(err.Error())
// 	}
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Group all processes per app
// 	processesPerApp := map[string][]cfclient.Process{}
// 	for _, process := range processes {
// 		parts := strings.Split(process.Links.App.Href, "/")
// 		appGUID := parts[len(parts)-1]
// 		appProcesses, exists := processesPerApp[appGUID]
// 		if exists {
// 			appProcesses = append(appProcesses, process)
// 		} else {
// 			appProcesses = []cfclient.Process{process}
// 		}
// 		processesPerApp[appGUID] = appProcesses
// 	}

// 	// Create a space Map
// 	spacesPerGUID := map[string]cfclient.V3Space{}
// 	for _, space := range spaces {
// 		spacesPerGUID[space.GUID] = space
// 	}

// 	// Create an org Map
// 	orgsPerGUID := map[string]cfclient.V3Organization{}
// 	for _, org := range orgs {
// 		orgsPerGUID[org.GUID] = org
// 	}

// 	// Populate CFApplication
// 	results := []CFApplication{}
// 	for _, cfapp := range cfapps {
// 		updatedApp := cfapp
// 		appGUID := cfapp.GUID
// 		spaceGUID := cfapp.SpaceGUID
// 		processes, exists := processesPerApp[appGUID]
// 		if exists {
// 			updatedApp.setV3ProcessData(processes)
// 		} else {
// 			cfc.logger.Errorf("could not fetch processes info for app guid %s", appGUID)
// 		}
// 		// Fill space then org data. Order matters for labels and annotations.
// 		space, exists := spacesPerGUID[spaceGUID]
// 		if exists {
// 			updatedApp.setV3SpaceData(space)
// 		} else {
// 			cfc.logger.Errorf("could not fetch space info for space guid %s", spaceGUID)
// 		}
// 		orgGUID := updatedApp.OrgGUID
// 		org, exists := orgsPerGUID[orgGUID]
// 		if exists {
// 			updatedApp.setV3OrgData(org)
// 		} else {
// 			cfc.logger.Errorf("could not fetch org info for org guid %s", orgGUID)
// 		}
// 		results = append(results, updatedApp)
// 	}

// 	return results, nil
// }

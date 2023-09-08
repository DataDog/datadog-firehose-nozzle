// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

// DCAClient is required to query the API of Datadog cluster agent
type DCAClient struct {
	clusterAgentAPIEndpoint       string  // ${SCHEME}://${clusterAgentHost}:${PORT}
	ClusterAgentVersion           Version // Version of the cluster-agent we're connected to
	clusterAgentAPIClient         *http.Client
	clusterAgentAPIRequestHeaders http.Header
	logger                        *gosteno.Logger
	advancedTagging               bool
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
	dcaClient.clusterAgentAPIRequestHeaders.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	dcaClient.clusterAgentAPIClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 2 * time.Second,
	}

	// Validate the cluster agent client by checking the version
	dcaClient.ClusterAgentVersion, err = dcaClient.GetVersion()
	if err != nil {
		ticker := time.NewTicker(5 * time.Second)
		nbrAttempts := 10
	RetryLoop:
		for {
			select {
			case <-ticker.C:
				dcaClient.ClusterAgentVersion, err = dcaClient.GetVersion()
				if err == nil {
					break RetryLoop
				} else {
					logger.Warnf("Unsuccessful attempt to connect to the Datadog Cluster Agent: %s", err)
					nbrAttempts -= 1
					if nbrAttempts == 0 {
						logger.Errorf("Could not to connect to the Datadog Cluster Agent: %s", err)
						os.Exit(1)
					}
				}
			}
		}
	}

	logger.Infof("Successfully connected to the Datadog Cluster Agent %s", dcaClient.ClusterAgentVersion.String())

	return &dcaClient, nil
}

// Version returns ClusterAgentVersion already stored in the DCAClient
func (c *DCAClient) Version() Version {
	return c.ClusterAgentVersion
}

// GetVersion fetches the version of the Cluster Agent.
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

// GetApplications fetches the list of CF Applications from the Cluster Agent.
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

	for _, cfapp := range cfapps {
		if !c.advancedTagging {
			cfapp.Sidecars = nil
		} else {
			if cfapp.Sidecars == nil {
				cfapp.Sidecars = make([]CFSidecar, 0)
			}
		}
	}

	return cfapps, err
}

// GetApplication fetches a CF Application with the given appGUID from the Cluster Agent.
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

	if !c.advancedTagging {
		cfapp.Sidecars = nil
	} else {
		if cfapp.Sidecars == nil {
			cfapp.Sidecars = make([]CFSidecar, 0)
		}
	}

	return &cfapp, err
}

// // GetV3Orgs fetches a V3 CF Organizations from the Cluster Agent.
func (c *DCAClient) GetV3Orgs() ([]cfclient.V3Organization, error) {
	const dcaOrgsPath = "api/v1/cf/orgs"
	var allOrgs []cfclient.V3Organization
	var err error

	// https://host:port/api/v1/cf/orgs
	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaOrgsPath)

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

	err = json.Unmarshal(body, &allOrgs)

	return allOrgs, err
}

// GetV2OrgQuotas fetches CF Organization Quotas from the Cluster Agent.
func (c *DCAClient) GetV2OrgQuotas() ([]CFOrgQuota, error) {
	const dcaOrgQuotasPath = "api/v1/cf/org_quotas"
	var allQuotas []CFOrgQuota
	var err error

	// https://host:port/api/v1/cf/org_quotas
	rawURL := fmt.Sprintf("%s/%s", c.clusterAgentAPIEndpoint, dcaOrgQuotasPath)

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

	err = json.Unmarshal(body, &allQuotas)

	return allQuotas, err
}

// V2OrgsFromV3Orgs
func (c *DCAClient) V2OrgsFromV3Orgs() ([]cfclient.Org, error) {
	allOrgs, err := c.GetV3Orgs()
	if err != nil {
		return nil, err
	}

	var allV2Orgs []cfclient.Org

	for _, v3Org := range allOrgs {
		allV2Orgs = append(allV2Orgs, cfclient.Org{
			Name:                v3Org.Name,
			Guid:                v3Org.GUID,
			CreatedAt:           v3Org.CreatedAt,
			UpdatedAt:           v3Org.UpdatedAt,
			QuotaDefinitionGuid: v3Org.Relationships["quota"].Data.GUID,
		})
	}

	return allV2Orgs, nil
}

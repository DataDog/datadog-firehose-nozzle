package cloudfoundry

import (
	"fmt"
	"strconv"
	"strings"
	"net/url"
	"io/ioutil"
	"math"
	"sync"
	"encoding/json"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/pkg/errors"
)

type CFClient struct {
	apiVersion 	int
	numWorkers	int
	client 		*cfclient.Client
	logger 		*gosteno.Logger
}

// CFApplication represents a Cloud Controller Application.
type CFApplication struct {
	GUID string
	Name string
	SpaceGUID string
	SpaceName string
	OrgName string
	OrgGUID string
	Instances int
	Buildpacks []string
	DiskQuota int
	TotalDiskQuota int
	Memory int
	TotalMemory int
}

type Data struct {
	Data struct {
		Guid string `json:"guid"`
	} `json:"data"`
}

type v3AppResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources []v3AppResource `json:"resources"`
}

type v3AppResource struct {
	Guid                     string                 `json:"guid"`
	Name                     string                 `json:"name"`
	State                    string                 `json:"state"`
	CreatedAt                string                 `json:"created_at"`
	UpdatedAt                string                 `json:"updated_at"`
	LifeCycle struct {
		Type string `json:"type"`
		Data struct {
			BuildPacks  []string 	`json:"buildpacks"`
			Stack 		string    	`json:"stack"`
		} `json:"data"`
	} `json:"lifecycle"`
	Relationships struct { Space Data `json:"space"` } `json:"relationships"`
	Links struct {
		Self 					cfclient.Link 	`json:"self"`
		Space 					cfclient.Link 	`json:"space"`
		Processes 				cfclient.Link 	`json:"processes"`
		Packages 				cfclient.Link 	`json:"packages"`
		EnvironmentVariables 	cfclient.Link 	`json:"environment_variables"`
		CurrentDroplet 			cfclient.Link 	`json:"current_droplet"`
		Droplets 				cfclient.Link 	`json:"droplets"`
		Tasks 					cfclient.Link 	`json:"tasks"`
		Start					cfclient.Link 	`json:"start"`
		Stop					cfclient.Link   `json:"stop"`
		Revisions				cfclient.Link 	`json:"revisions"`
		DeployedRevisions		cfclient.Link 	`json:"deployed_revisions"`
	} `json:"links"`
}

type v3SpaceResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources []v3SpaceResource `json:"resources"`
}

type v3SpaceResource struct {
	Guid        string	`json:"guid"`
	Name		string 	`json:"name"`
	CreatedAt	string	`json:"created_at"`
	UpdatedAt	string	`json:"updated_at"`
	Relationships struct { Organization Data `json:"organization"` } `json:"relationships"`
	Links struct {
		Self 					cfclient.Link 	`json:"self"`
		Organization 			cfclient.Link 	`json:"organization"`
	} `json:"links"`
}

func NewClient(config *config.Config, logger *gosteno.Logger) (*CFClient, error) {
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

	cfc := CFClient{
		apiVersion: 0,
		numWorkers: config.NumWorkers,
		client: cfClient,
		logger: logger,
	}
	return &cfc, nil
}

func (cfc *CFClient) GetDopplerEndpoint() string {
	return cfc.client.Endpoint.DopplerEndpoint
}

func (cfc *CFClient) GetOrganizationsQuotas(numWorkers int) ([]cfclient.OrgQuotasResource, error) {
	results, pages, err := cfc.getV2OrganizationsQuotasByPage(1)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting apps page 1, skipping cache warmup")
	}

	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Calculate the number of workers needs based on the number of pages found
	pages = pages - 1 // We already have the first page
	numWorkers = int(math.Min(float64(numWorkers), float64(pages))) // We cannot have more workers than pages to fetch
	var pagesPerWorker int
	if pages > 0 {
		pagesPerWorker = int(math.Ceil(float64(pages) / float64(numWorkers)))
	}
	// Use go routines to fetch page ranges
	for worker := 0; worker < numWorkers; worker++ {
		// Offset 2 because no page at index 0 and page 1 already fetched
		start := worker * pagesPerWorker + 2
		// Stop at page pages + 1, to get the last one
		end := int(math.Min(float64((worker + 1) * pagesPerWorker + 2), float64(pages)))
		if end == pages {
			// Add 1 so the last page can be obtained
			end++
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()

			for currentPage := start; currentPage < end; currentPage++ {
				pageResults, _, err := cfc.getV2OrganizationsQuotasByPage(currentPage)
				if err != nil {
					cfc.logger.Error(err.Error())
					continue
				}
				mutex.Lock()
				results = append(results, pageResults...)
				mutex.Unlock()
			}
		}(start, end)
	}
	wg.Wait()

	return results, nil
}

func (cfc *CFClient) GetApplications() ([]CFApplication, error) {
	if cfc.apiVersion == 2 {
		return cfc.getV2Applications()
	}
	if cfc.apiVersion == 3 {
		return cfc.getV3Applications()
	}

	results, err := cfc.getV3Applications()
	if err != nil{
		results, err = cfc.getV2Applications()
		if err != nil{
			return nil, err
		}
		cfc.apiVersion = 2
	}
	cfc.apiVersion = 3
	return results, nil
}

func (cfc *CFClient) GetApplication(guid string) (*CFApplication, error) {
	app, err := cfc.client.GetAppByGuid(guid)
	if err != nil{
		return nil, err
	}
	result := CFApplication{}
	result.setV2AppData(app)
	return &result, nil
}

func (cfc *CFClient) getV2OrganizationsQuotasByPage(page int) ([]cfclient.OrgQuotasResource, int, error) {
	//NOTE: Taken from https://github.com/cloudfoundry-community/go-cfclient/blob/16c98753d3152f9d80d3c121523536858095a3da/apps.go#L332
	q := url.Values{}
	q.Set("results-per-page", "100") // 100 is the max
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	r := cfc.client.NewRequest("GET", "/v2/quota_definitions?"+q.Encode())
	resp, err := cfc.client.DoRequest(r)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error requesting quota_definitions page %d", page)
	}
	// Read body response
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error reading app response for quota_definition %d", page)
	}
	// Unmarshal body response into OrgQuotasResponse objects
	var orgsResp cfclient.OrgQuotasResponse
	err = json.Unmarshal(resBody, &orgsResp)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error unmarshalling app response for quota_definition %d", page)
	}

	return orgsResp.Resources, orgsResp.Pages, nil
}

func (cfc *CFClient) getV3Applications() ([]CFApplication, error) {
	// Query the first page to get the total number of pages.
	cfapps, pages, err := cfc.getV3ApplicationsByPage(1)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting apps page 1, skipping cache warmup")
	}

	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Calculate the number of workers needs based on the number of pages found
	pages = pages - 1 // We already have the first page
	numWorkers := int(math.Min(float64(cfc.numWorkers), float64(pages))) // We cannot have more workers than pages to fetch
	var pagesPerWorker int
	if pages > 0 {
		pagesPerWorker = int(math.Ceil(float64(pages) / float64(numWorkers)))
	}
	// Use go routines to fetch page ranges
	for worker := 0; worker < numWorkers; worker++ {
		// Offset 2 because no page at index 0 and page 1 already fetched
		start := worker * pagesPerWorker + 2
		// Stop at page pages + 1, to get the last one
		end := int(math.Min(float64((worker + 1) * pagesPerWorker + 2), float64(pages)))
		if end == pages {
			// Add 1 so the last page can be obtained
			end++
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()

			for currentPage := start; currentPage < end; currentPage++ {
				pageResults, _, err := cfc.getV3ApplicationsByPage(currentPage)
				if err != nil {
					cfc.logger.Error(err.Error())
					continue
				}
				mutex.Lock()
				cfapps = append(cfapps, pageResults...)
				mutex.Unlock()
			}
		}(start, end)
	}
	wg.Wait()

	// Fetch processes
	processesPerApp := map[string][]cfclient.Process{}
	processes, err := cfc.getV3Processes()
	if err != nil {
		cfc.logger.Error(err.Error())
		return nil, err
	}
	// Group all processes per app
	for _, process := range processes {
		lastIndex := math.Max(float64(strings.LastIndex(process.Links.App.Href, "/")), float64(0))
		appGUID := process.Links.App.Href[int(lastIndex):]
		appProcesses, exists := processesPerApp[appGUID]
		if exists {
			appProcesses = append(appProcesses, process)
		}else{
			appProcesses = []cfclient.Process{process}
		}
		processesPerApp[appGUID] = appProcesses
	}

	// Fetch spaces
	spacesPerGuid := map[string]v3SpaceResource{}
	spaces, err := cfc.getV3Spaces()
	if err != nil {
		cfc.logger.Error(err.Error())
		return nil, err
	}
	// Create a space Map
	for _, space := range spaces {
		spacesPerGuid[space.Guid] = space
	}

	// Fetch spaces
	orgsPerGuid := map[string]cfclient.Org{}
	q := url.Values{}
	q.Set("results-per-page", "100") // 100 is the max
	orgs, err := cfc.client.ListOrgsByQuery(q)
	if err != nil {
		cfc.logger.Error(err.Error())
		return nil, err
	}
	// Create an org Map
	for _, org := range orgs {
		orgsPerGuid[org.Guid] = org
	}

	// Populate CFApplication
	results := []CFApplication{}
	for _, cfapp := range cfapps {
		updatedApp := cfapp
		appGUID := cfapp.GUID
		spaceGUID := cfapp.SpaceGUID
		processes, exists := processesPerApp[appGUID]
		if exists {
			updatedApp.setV3ProcessData(processes)
		}
		space, exists := spacesPerGuid[spaceGUID]
		if exists {
			updatedApp.setV3SpaceData(space)
		}
		orgGUID := updatedApp.OrgGUID
		org, exists := orgsPerGuid[orgGUID]
		if exists {
			updatedApp.setV3OrgData(org)
		}
		results = append(results, updatedApp)
	}

	return results, nil
}


func (cfc *CFClient) getV3ApplicationsByPage(page int) ([]CFApplication, int, error){
	q := url.Values{}
	q.Set("per-page", "5000") // 5000 is the max

	// Since we don't know what version of the v3 endpoint we use, we have to discover it.
	// Hence, we need to try for the latest to the oldest approach.
	// 1. try with include=space,space.organization (as supported in 3.77.0)
	// 2. try with include=space,org as supported in 3.76.0 all the way down to 3.73.0
	// 3. try with include=space only since it seems that org doesn't work on some versions
	//	- In this case we need to make a separate call to the org endpoint (ideally fetch quotas at the same time).
	// 4. try without include
	//  - In this case we need to make two more calls, on for org and one for space.
	//
	//if cfc.include != "" {
	//	q.Set("include", cfc.include)
	//}

	r := cfc.client.NewRequest("GET", "/v3/apps?" + q.Encode())
	resp, err := cfc.client.DoRequest(r)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error requesting apps page %d", err)
	}
	// Read body response
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error reading app response for page %d", page)
	}
	// Unmarshal body response into v3AppResponse objects
	var appResp v3AppResponse
	err = json.Unmarshal(resBody, &appResp)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error unmarshalling app response for page %d", page)
	}
	// Create CFApplication objects
	appResources := appResp.Resources
	results := []CFApplication{}
	for _, app := range appResources {
		cfapp := CFApplication{}
		cfapp.setV3AppData(app)
		results = append(results, cfapp)
	}

	return results, appResp.Pagination.TotalPages, nil
}

func (cfc *CFClient) getV3Processes() ([]cfclient.Process, error){
	q := url.Values{}
	q.Set("per_page", "5000") // 5000 is the max
	return cfc.client.ListAllProcessesByQuery(q)
}

func (cfc *CFClient) getV3Spaces() ([]v3SpaceResource, error) {
	// Query the first page to get the total number of pages.
	results, pages, err := cfc.getV3SpacesByPage(1)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting apps page 1, skipping cache warmup")
	}

	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Calculate the number of workers needs based on the number of pages found
	pages = pages - 1 // We already have the first page
	numWorkers := int(math.Min(float64(cfc.numWorkers), float64(pages))) // We cannot have more workers than pages to fetch
	var pagesPerWorker int
	if pages > 0 {
		pagesPerWorker = int(math.Ceil(float64(pages) / float64(numWorkers)))
	}
	// Use go routines to fetch page ranges
	for worker := 0; worker < numWorkers; worker++ {
		// Offset 2 because no page at index 0 and page 1 already fetched
		start := worker * pagesPerWorker + 2
		// Stop at page pages + 1, to get the last one
		end := int(math.Min(float64((worker + 1) * pagesPerWorker + 2), float64(pages)))
		if end == pages {
			// Add 1 so the last page can be obtained
			end++
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()

			for currentPage := start; currentPage < end; currentPage++ {
				pageResults, _, err := cfc.getV3SpacesByPage(currentPage)
				if err != nil {
					cfc.logger.Error(err.Error())
					continue
				}
				mutex.Lock()
				results = append(results, pageResults...)
				mutex.Unlock()
			}
		}(start, end)
	}
	wg.Wait()

	return results, nil
}

func (cfc *CFClient) getV3SpacesByPage(page int) ([]v3SpaceResource, int, error){
	//NOTE: Taken from https://github.com/cloudfoundry-community/go-cfclient/blob/16c98753d3152f9d80d3c121523536858095a3da/apps.go#L332
	q := url.Values{}
	q.Set("per-page", "5000") // 5000 is the max

	r := cfc.client.NewRequest("GET", "/v3/spaces?" + q.Encode())
	resp, err := cfc.client.DoRequest(r)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error requesting spaces page %d", err)
	}
	// Read body response
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error reading space response for page %d", page)
	}
	// Unmarshal body response into v3SpaceResponse objects
	var spaceRes v3SpaceResponse
	err = json.Unmarshal(resBody, &spaceRes)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error unmarshalling space response for page %d", page)
	}

	return spaceRes.Resources, spaceRes.Pagination.TotalPages, nil
}

func (cfc *CFClient) getV2Applications() ([]CFApplication, error) {
	// Query the first page to get the total number of pages.
	results, pages, err := cfc.getV2ApplicationsByPage(1)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting apps page 1, skipping cache warmup")
	}

	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Calculate the number of workers needs based on the number of pages found
	pages = pages - 1 // We already have the first page
	numWorkers := int(math.Min(float64(cfc.numWorkers), float64(pages))) // We cannot have more workers than pages to fetch
	var pagesPerWorker int
	if pages > 0 {
		pagesPerWorker = int(math.Ceil(float64(pages) / float64(numWorkers)))
	}
	// Use go routines to fetch page ranges
	for worker := 0; worker < numWorkers; worker++ {
		// Offset 2 because no page at index 0 and page 1 already fetched
		start := worker * pagesPerWorker + 2
		// Stop at page pages + 1, to get the last one
		end := int(math.Min(float64((worker + 1) * pagesPerWorker + 2), float64(pages)))
		if end == pages {
			// Add 1 so the last page can be obtained
			end++
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()

			for currentPage := start; currentPage < end; currentPage++ {
				pageResults, _, err := cfc.getV2ApplicationsByPage(currentPage)
				if err != nil {
					cfc.logger.Error(err.Error())
					continue
				}
				mutex.Lock()
				results = append(results, pageResults...)
				mutex.Unlock()
			}
		}(start, end)
	}
	wg.Wait()

	return results, nil
}

func (cfc *CFClient) getV2ApplicationsByPage(page int) ([]CFApplication, int, error) {
	q := url.Values{}
	q.Set("inline-relations-depth", "2")
	q.Set("results-per-page", "100") // 100 is the max
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	r := cfc.client.NewRequest("GET", "/v2/apps?"+q.Encode())
	resp, err := cfc.client.DoRequest(r)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error requesting apps page %d", page)
	}
	// Read body response
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error reading app response for page %d", page)
	}
	// Unmarshal body response into AppResponse objects
	var appResp cfclient.AppResponse
	err = json.Unmarshal(resBody, &appResp)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error unmarshalling app response for page %d", page)
	}
	// Create CFApplication objects
	appResources := appResp.Resources
	results := []CFApplication{}
	for _, app := range appResources {
		app.Entity.Guid = app.Meta.Guid
		app.Entity.CreatedAt = app.Meta.CreatedAt
		app.Entity.UpdatedAt = app.Meta.UpdatedAt
		app.Entity.SpaceData.Entity.Guid = app.Entity.SpaceData.Meta.Guid
		app.Entity.SpaceData.Entity.OrgData.Entity.Guid = app.Entity.SpaceData.Entity.OrgData.Meta.Guid
		cfapp := CFApplication{}
		cfapp.setV2AppData(app.Entity)
		results = append(results, cfapp)
	}

	return results, appResp.Pages, nil
}

func (a *CFApplication) setV2AppData(data cfclient.App) {
	a.GUID = data.Guid
	a.Name = data.Name

	a.SpaceGUID = data.SpaceGuid
	a.Instances = data.Instances

	a.DiskQuota = data.DiskQuota
	a.Memory = data.Memory
	a.TotalDiskQuota = data.DiskQuota * a.Instances
	a.TotalMemory = data.Memory * a.Instances

	a.SpaceName = data.SpaceData.Entity.Name
	a.OrgName = data.SpaceData.Entity.OrgData.Entity.Name
	a.OrgGUID = data.SpaceData.Entity.OrgData.Entity.Guid
}

func (a *CFApplication) setV3AppData(data v3AppResource) {
	a.GUID = data.Guid
	a.Name = data.Name
	a.SpaceGUID = data.Relationships.Space.Data.Guid
	a.Buildpacks = data.LifeCycle.Data.BuildPacks
}

func (a *CFApplication) setV3ProcessData(data []cfclient.Process) {
	if len(data) <= 0 {
		return
	}
	totalInstances := 0
	totalDiskInMbConfigured := 0
	totalDiskInMbProvisioned := 0
	totalMemoryInMbConfigured := 0
	totalMemoryInMbProvisioned := 0

	for _, p := range data {
		instances := p.Instances
		diskInMbConfigured := p.DiskInMB
		diskInMbProvisioned := instances * diskInMbConfigured
		memoryInMbConfigured := p.MemoryInMB
		memoryInMbProvisioned := p.MemoryInMB * memoryInMbConfigured

		totalInstances += instances
		totalDiskInMbConfigured += diskInMbConfigured
		totalDiskInMbProvisioned += diskInMbProvisioned
		totalMemoryInMbConfigured += memoryInMbConfigured
		totalMemoryInMbProvisioned += memoryInMbProvisioned
	}

	a.Instances = totalInstances

	a.DiskQuota = totalDiskInMbConfigured
	a.Memory = totalMemoryInMbConfigured
	a.TotalDiskQuota = totalDiskInMbProvisioned
	a.TotalMemory = totalMemoryInMbProvisioned
}

func (a *CFApplication) setV3SpaceData(data v3SpaceResource) {
	a.SpaceName = data.Name
	a.OrgGUID = data.Relationships.Organization.Data.Guid
}

func (a *CFApplication) setV3OrgData(data cfclient.Org) {
	a.OrgName = data.Name
}

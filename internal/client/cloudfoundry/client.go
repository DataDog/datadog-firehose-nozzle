package cloudfoundry

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/pkg/errors"
)

type CFClient struct {
	ApiVersion   int
	NumWorkers   int
	client       *cfclient.Client
	logger       *gosteno.Logger
	apiBatchSize string
}

// CFApplication represents a Cloud Controller Application.
type CFApplication struct {
	GUID           string
	Name           string
	SpaceGUID      string
	SpaceName      string
	OrgName        string
	OrgGUID        string
	Instances      int
	Buildpacks     []string
	DiskQuota      int
	TotalDiskQuota int
	Memory         int
	TotalMemory    int
}

type Data struct {
	Data struct {
		GUID string `json:"guid"`
	} `json:"data"`
}

type v3AppResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources  []v3AppResource     `json:"resources"`
}

type v3AppResource struct {
	GUID      string `json:"guid"`
	Name      string `json:"name"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	LifeCycle struct {
		Type string `json:"type"`
		Data struct {
			BuildPacks []string `json:"buildpacks"`
			Stack      string   `json:"stack"`
		} `json:"data"`
	} `json:"lifecycle"`
	Relationships struct {
		Space Data `json:"space"`
	} `json:"relationships"`
	Links struct {
		Self                 cfclient.Link `json:"self"`
		Space                cfclient.Link `json:"space"`
		Processes            cfclient.Link `json:"processes"`
		Packages             cfclient.Link `json:"packages"`
		EnvironmentVariables cfclient.Link `json:"environment_variables"`
		CurrentDroplet       cfclient.Link `json:"current_droplet"`
		Droplets             cfclient.Link `json:"droplets"`
		Tasks                cfclient.Link `json:"tasks"`
		Start                cfclient.Link `json:"start"`
		Stop                 cfclient.Link `json:"stop"`
		RouteMappings        cfclient.Link `json:"route_mappings,omitempty"`
		Revisions            cfclient.Link `json:"revisions,omitempty"`
		DeployedRevisions    cfclient.Link `json:"deployed_revisions,omitempty"`
	} `json:"links"`
}

type v3SpaceResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources  []v3SpaceResource   `json:"resources"`
}

type v3SpaceResource struct {
	GUID          string `json:"guid"`
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	Relationships struct {
		Organization Data `json:"organization"`
	} `json:"relationships"`
	Links struct {
		Self         cfclient.Link `json:"self"`
		Organization cfclient.Link `json:"organization"`
	} `json:"links"`
}

type v3OrgResponse struct {
	Pagination cfclient.Pagination `json:"pagination"`
	Resources  []cfclient.Org      `json:"resources"`
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
		ApiVersion:   0,
		NumWorkers:   config.NumWorkers,
		client:       cfClient,
		logger:       logger,
		apiBatchSize: fmt.Sprint(config.CloudControllerAPIBatchSize),
	}
	return &cfc, nil
}

func (cfc *CFClient) GetDopplerEndpoint() string {
	return cfc.client.Endpoint.DopplerEndpoint
}

func (cfc *CFClient) GetApplications() ([]CFApplication, error) {
	if cfc.ApiVersion == 2 {
		cfc.logger.Debug("api version is 2")
		return cfc.getV2Applications()
	}
	if cfc.ApiVersion == 3 {
		cfc.logger.Debug("api version is 3")
		return cfc.getV3Applications()
	}
	cfc.logger.Debug("no api version set, trying to collect data with version 3")
	results, err := cfc.getV3Applications()
	if err != nil {
		cfc.logger.Debug("error trying to fetch application infos with v3 endpoints. Falling back to v2 endpoints")
		results, err = cfc.getV2Applications()
		if err != nil {
			cfc.logger.Errorf("error trying to fetch application infos with v2 endpoints %v", err)
			return nil, err
		}
		cfc.ApiVersion = 2
	}
	cfc.ApiVersion = 3
	return results, nil
}

func (cfc *CFClient) GetApplication(guid string) (*CFApplication, error) {
	app, err := cfc.client.GetAppByGuid(guid)
	if err != nil {
		return nil, err
	}
	result := CFApplication{}
	result.setV2AppData(app)
	return &result, nil
}

func (cfc *CFClient) getV3Applications() ([]CFApplication, error) {
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	// Fetch apps
	wg.Add(1)
	var cfapps []CFApplication
	go func() {
		defer wg.Done()
		var err error
		cfapps, err = cfc.getV3Apps()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch processes
	wg.Add(1)
	var processes []cfclient.Process
	go func() {
		defer wg.Done()
		var err error
		processes, err = cfc.getV3Processes()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch spaces
	wg.Add(1)
	var spaces []v3SpaceResource
	go func() {
		defer wg.Done()
		var err error
		spaces, err = cfc.getV3Spaces()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch orgs
	wg.Add(1)
	var orgs []cfclient.Org
	go func() {
		defer wg.Done()
		var err error
		orgs, err = cfc.getV3Orgs()
		if err != nil {
			errors <- err
		}
	}()

	wg.Wait()
	close(errors)

	// Go through the channel, print all errors and return one of them if there are any
	var err error
	for err = range errors {
		cfc.logger.Error(err.Error())
	}
	if err != nil {
		return nil, err
	}

	// Group all processes per app
	processesPerApp := map[string][]cfclient.Process{}
	for _, process := range processes {
		parts := strings.Split(process.Links.App.Href, "/")
		appGUID := parts[len(parts)-1]
		appProcesses, exists := processesPerApp[appGUID]
		if exists {
			appProcesses = append(appProcesses, process)
		} else {
			appProcesses = []cfclient.Process{process}
		}
		processesPerApp[appGUID] = appProcesses
	}

	// Create a space Map
	spacesPerGUID := map[string]v3SpaceResource{}
	for _, space := range spaces {
		spacesPerGUID[space.GUID] = space
	}

	// Create an org Map
	orgsPerGUID := map[string]cfclient.Org{}
	for _, org := range orgs {
		orgsPerGUID[org.Guid] = org
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
		} else {
			cfc.logger.Errorf("could not fetch processes info for app guid %s", appGUID)
		}
		space, exists := spacesPerGUID[spaceGUID]
		if exists {
			updatedApp.setV3SpaceData(space)
		} else {
			cfc.logger.Errorf("could not fetch space info for space guid %s", spaceGUID)
		}
		orgGUID := updatedApp.OrgGUID
		org, exists := orgsPerGUID[orgGUID]
		if exists {
			updatedApp.setV3OrgData(org)
		} else {
			cfc.logger.Errorf("could not fetch org info for org guid %s", orgGUID)
		}
		results = append(results, updatedApp)
	}

	return results, nil
}

func (cfc *CFClient) getV3Apps() ([]CFApplication, error) {
	var cfapps []CFApplication

	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", cfc.apiBatchSize)
		q.Set("page", strconv.Itoa(page))
		r := cfc.client.NewRequest("GET", "/v3/apps?"+q.Encode())
		resp, err := cfc.client.DoRequest(r)
		if err != nil {
			return nil, errors.Wrapf(err, "Error requesting apps page %d", err)
		}
		// Read body response
		defer resp.Body.Close()
		resBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "Error reading app response for page %d", page)
		}
		// Unmarshal body response into v3AppResponse objects
		var appResp v3AppResponse
		err = json.Unmarshal(resBody, &appResp)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling app response for page %d", page)
		}
		// Create CFApplication objects
		appResources := appResp.Resources
		for _, app := range appResources {
			cfapp := CFApplication{}
			cfapp.setV3AppData(app)
			cfapps = append(cfapps, cfapp)
		}
		if appResp.Pagination.TotalPages <= page {
			break
		}
	}

	return cfapps, nil
}

func (cfc *CFClient) getV3Processes() ([]cfclient.Process, error) {
	// Query the first page to get the total number of pages.
	var cfprocesses []cfclient.Process
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", cfc.apiBatchSize)
		q.Set("page", strconv.Itoa(page))
		r := cfc.client.NewRequest("GET", "/v3/processes?"+q.Encode())
		resp, err := cfc.client.DoRequest(r)
		if err != nil {
			return nil, errors.Wrapf(err, "Error requesting v3 processes page %d", page)
		}
		// Read body response
		defer resp.Body.Close()
		resBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "Error reading v3 processes response for page %d", page)
		}
		// Unmarshal body response into ProcessListResponse objects
		var processResp cfclient.ProcessListResponse
		err = json.Unmarshal(resBody, &processResp)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling v3 processes response for page %d", page)
		}
		cfprocesses = append(cfprocesses, processResp.Processes...)
		if processResp.Pagination.TotalPages <= page {
			break
		}
	}

	return cfprocesses, nil
}

func (cfc *CFClient) getV3Spaces() ([]v3SpaceResource, error) {
	var spaces []v3SpaceResource

	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", cfc.apiBatchSize)
		q.Set("page", strconv.Itoa(page))
		r := cfc.client.NewRequest("GET", "/v3/spaces?"+q.Encode())
		resp, err := cfc.client.DoRequest(r)
		if err != nil {
			return nil, errors.Wrapf(err, "Error requesting v3 spaces page %d", page)
		}
		// Read body response
		defer resp.Body.Close()
		resBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "Error reading v3 spaces response for page %d", page)
		}
		// Unmarshal body response into v3SpaceResponse object
		var spaceResp v3SpaceResponse
		err = json.Unmarshal(resBody, &spaceResp)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling v3 spaces response for page %d", page)
		}
		spaces = append(spaces, spaceResp.Resources...)
		if spaceResp.Pagination.TotalPages <= page {
			break
		}
	}

	return spaces, nil
}

func (cfc *CFClient) getV3Orgs() ([]cfclient.Org, error) {
	var cforgs []cfclient.Org

	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("per_page", cfc.apiBatchSize)
		q.Set("page", strconv.Itoa(page))
		r := cfc.client.NewRequest("GET", "/v3/organizations?"+q.Encode())
		resp, err := cfc.client.DoRequest(r)
		if err != nil {
			return nil, errors.Wrapf(err, "Error requesting v3 orgs page %d", page)
		}
		// Read body response
		defer resp.Body.Close()
		resBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "Error reading v3 orgs response for page %d", page)
		}
		// Unmarshal body response into  objects
		var orgsResp v3OrgResponse
		err = json.Unmarshal(resBody, &orgsResp)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling v3 orgs response for page %d", page)
		}
		cforgs = append(cforgs, orgsResp.Resources...)
		if orgsResp.Pagination.TotalPages <= page {
			break
		}
	}

	return cforgs, nil
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
	numWorkers := int(math.Min(float64(cfc.NumWorkers), float64(pages))) // We cannot have more workers than pages to fetch
	var pagesPerWorker int
	if pages-1 > 0 { // We already have the first page
		pagesPerWorker = int(math.Ceil(float64(pages-1) / float64(numWorkers)))
	}
	// Use go routines to fetch page ranges
	for worker := 0; worker < numWorkers; worker++ {
		// Offset 2 because no page at index 0 and page 1 already fetched
		start := (worker * pagesPerWorker) + 2
		// Stop at page pages + pageWindow, to get the last one
		end := int(math.Min(float64((worker+1)*pagesPerWorker+2), float64(pages+1)))
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
		return nil, -1, errors.Wrapf(err, "Error requesting v2 apps page %d", page)
	}
	// Read body response
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error reading v2 app response for page %d", page)
	}
	// Unmarshal body response into AppResponse objects
	var appResp cfclient.AppResponse
	err = json.Unmarshal(resBody, &appResp)
	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error unmarshalling v2 app response for page %d", page)
	}
	// Create CFApplication objects
	appResources := appResp.Resources
	results := []CFApplication{}
	for _, app := range appResources {
		app.Entity.Guid = app.Meta.Guid
		app.Entity.CreatedAt = app.Meta.CreatedAt
		app.Entity.UpdatedAt = app.Meta.UpdatedAt
		cfapp := CFApplication{}
		cfapp.setV2AppData(app.Entity)
		results = append(results, cfapp)
	}

	return results, appResp.Pages, nil
}

func (cfc *CFClient) GetV2Orgs() ([]cfclient.Org, error) {
	query := url.Values{}
	query.Set("results-per-page", "100")

	allOrgs, err := cfc.client.ListOrgsByQuery(query)
	if err != nil {
		return nil, err
	}

	return allOrgs, nil
}

func (cfc *CFClient) GetV2OrgQuotas() ([]cfclient.OrgQuota, error) {
	query := url.Values{}
	query.Set("results-per-page", "100")

	allQuotas, err := cfc.client.ListOrgQuotasByQuery(query)
	if err != nil {
		return nil, err
	}

	return allQuotas, nil
}

func (a *CFApplication) setV2AppData(data cfclient.App) {
	a.GUID = data.Guid
	a.Name = data.Name

	a.SpaceGUID = data.SpaceData.Meta.Guid
	a.SpaceName = data.SpaceData.Entity.Name

	a.OrgName = data.SpaceData.Entity.OrgData.Entity.Name
	a.OrgGUID = data.SpaceData.Entity.OrgData.Meta.Guid

	a.Instances = data.Instances
	a.DiskQuota = data.DiskQuota
	a.Memory = data.Memory
	a.TotalDiskQuota = data.DiskQuota * a.Instances
	a.TotalMemory = data.Memory * a.Instances

	a.Buildpacks = []string{}
	if data.Buildpack != "" {
		a.Buildpacks = append(a.Buildpacks, data.Buildpack)
	} else if data.DetectedBuildpack != "" {
		a.Buildpacks = append(a.Buildpacks, data.DetectedBuildpack)
	}
}

func (a *CFApplication) setV3AppData(data v3AppResource) {
	a.GUID = data.GUID
	a.Name = data.Name
	a.SpaceGUID = data.Relationships.Space.Data.GUID
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
		memoryInMbProvisioned := instances * memoryInMbConfigured

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
	a.OrgGUID = data.Relationships.Organization.Data.GUID
}

func (a *CFApplication) setV3OrgData(data cfclient.Org) {
	a.OrgName = data.Name
}

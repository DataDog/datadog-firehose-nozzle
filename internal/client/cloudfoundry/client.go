package cloudfoundry

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	cfclient "github.com/cloudfoundry/go-cfclient/v3/client"
	cfconfig "github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/cloudfoundry/gosteno"
)

const (
	// unlimitedQuota is how the Cloud Controller stores unlimited quotas; the v3 API renders it as null.
	unlimitedQuota = -1
	// ccRequestTimeout bounds each Cloud Controller request, page reads included. It matches the gorouter's default
	// backend timeout (router.request_timeout_in_seconds): go-cfclient's 30s default can fail large pages that the
	// router, and the previous unbounded client, allow.
	ccRequestTimeout = 900 * time.Second
)

type CFClient struct {
	client          *cfclient.Client
	logger          *gosteno.Logger
	apiBatchSize    uint32
	advancedTagging bool
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
	Labels         map[string]string
	Annotations    map[string]string
	Sidecars       []CFSidecar
}

type CFSidecar struct {
	Name string
	GUID string
}

type CFOrgQuota struct {
	GUID        string
	MemoryLimit int
}

func NewClient(config *config.Config, logger *gosteno.Logger) (*CFClient, error) {
	if config.CloudControllerEndpoint == "" {
		logger.Warnf("the Cloud Controller Endpoint needs to be set in order to set up the cf client")
		return nil, fmt.Errorf("the Cloud Controller Endpoint needs to be set in order to set up the cf client")
	}

	options := []cfconfig.Option{
		cfconfig.ClientCredentials(config.Client, config.ClientSecret),
		cfconfig.UserAgent("datadog-firehose-nozzle"),
		cfconfig.RequestTimeout(ccRequestTimeout),
	}
	if config.InsecureSSLSkipVerify {
		options = append(options, cfconfig.SkipTLSValidation())
	}
	cfg, err := cfconfig.New(config.CloudControllerEndpoint, options...)
	if err != nil {
		logger.Warnf("encountered an error while setting up the cf client: %v", err)
		return nil, fmt.Errorf("encountered an error while setting up the cf client: %v", err)
	}
	cfClient, err := cfclient.New(cfg)
	if err != nil {
		logger.Warnf("encountered an error while setting up the cf client: %v", err)
		return nil, fmt.Errorf("encountered an error while setting up the cf client: %v", err)
	}

	cfc := CFClient{
		client:          cfClient,
		logger:          logger,
		apiBatchSize:    config.CloudControllerAPIBatchSize,
		advancedTagging: config.EnableAdvancedTagging,
	}
	return &cfc, nil
}

func (cfc *CFClient) GetApplications() ([]CFApplication, error) {
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	// Fetch apps
	wg.Add(1)
	var cfapps []CFApplication
	go func() {
		defer wg.Done()
		var err error
		cfapps, err = cfc.getApps()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch processes
	wg.Add(1)
	var processes []*resource.Process
	go func() {
		defer wg.Done()
		var err error
		processes, err = cfc.getProcesses()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch spaces
	wg.Add(1)
	var spaces []*resource.Space
	go func() {
		defer wg.Done()
		var err error
		spaces, err = cfc.getSpaces()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch orgs
	wg.Add(1)
	var orgs []*resource.Organization
	go func() {
		defer wg.Done()
		var err error
		orgs, err = cfc.GetOrgs()
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
	processesPerApp := map[string][]*resource.Process{}
	for _, process := range processes {
		appGUID := path.Base(process.Links["app"].Href)
		processesPerApp[appGUID] = append(processesPerApp[appGUID], process)
	}

	// Create a space Map
	spacesPerGUID := map[string]*resource.Space{}
	for _, space := range spaces {
		spacesPerGUID[space.GUID] = space
	}

	// Create an org Map
	orgsPerGUID := map[string]*resource.Organization{}
	for _, org := range orgs {
		orgsPerGUID[org.GUID] = org
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
		// Fill space then org data. Order matters for labels and annotations.
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

// GetApplication avoids the go-cfclient include helpers: they index the included resources without checking,
// which panics on an unexpected response.
func (cfc *CFClient) GetApplication(guid string) (*CFApplication, error) {
	ctx := context.Background()
	app, err := cfc.client.Applications.Get(ctx, guid)
	if err != nil {
		return nil, fmt.Errorf("Error requesting app %s: %w", guid, err)
	}
	if app.GUID == "" {
		return nil, fmt.Errorf("Error requesting app %s: empty response", guid)
	}

	result := CFApplication{}
	result.setV3AppData(app)
	if cfc.advancedTagging {
		result.Sidecars, err = cfc.getCFSidecars(guid)
		if err != nil {
			return nil, fmt.Errorf("Error reading sidecars response for app %s: %w", guid, err)
		}
	}

	processes, err := cfc.client.Processes.ListForAppAll(ctx, guid, &cfclient.ProcessListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting processes for app %s: %w", guid, err)
	}
	result.setV3ProcessData(processes)

	// Fill space then org data. Order matters for labels and annotations.
	space, err := cfc.client.Spaces.Get(ctx, result.SpaceGUID)
	if err != nil {
		return nil, fmt.Errorf("Error requesting space %s for app %s: %w", result.SpaceGUID, guid, err)
	}
	result.setV3SpaceData(space)

	org, err := cfc.client.Organizations.Get(ctx, result.OrgGUID)
	if err != nil {
		return nil, fmt.Errorf("Error requesting org %s for app %s: %w", result.OrgGUID, guid, err)
	}
	result.setV3OrgData(org)

	return &result, nil
}

func (cfc *CFClient) listOptions() *cfclient.ListOptions {
	return &cfclient.ListOptions{Page: 1, PerPage: int(cfc.apiBatchSize)}
}

func (cfc *CFClient) getApps() ([]CFApplication, error) {
	apps, err := cfc.client.Applications.ListAll(context.Background(), &cfclient.AppListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting apps: %w", err)
	}

	cfapps := make([]CFApplication, 0, len(apps))
	for _, app := range apps {
		cfapp := CFApplication{}
		cfapp.setV3AppData(app)
		if cfc.advancedTagging {
			sidecars, err := cfc.getCFSidecars(cfapp.GUID)
			if err != nil {
				return nil, fmt.Errorf("Error reading sidecars response for app %s: %w", cfapp.GUID, err)
			}
			cfapp.Sidecars = sidecars
		}
		cfapps = append(cfapps, cfapp)
	}

	return cfapps, nil
}

func (cfc *CFClient) getProcesses() ([]*resource.Process, error) {
	processes, err := cfc.client.Processes.ListAll(context.Background(), &cfclient.ProcessListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting v3 processes: %w", err)
	}
	return processes, nil
}

func (cfc *CFClient) getSpaces() ([]*resource.Space, error) {
	spaces, err := cfc.client.Spaces.ListAll(context.Background(), &cfclient.SpaceListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting v3 spaces: %w", err)
	}
	return spaces, nil
}

func (cfc *CFClient) GetOrgs() ([]*resource.Organization, error) {
	orgs, err := cfc.client.Organizations.ListAll(context.Background(), &cfclient.OrganizationListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting v3 orgs: %w", err)
	}
	return orgs, nil
}

func (cfc *CFClient) GetOrgQuotas() ([]CFOrgQuota, error) {
	quotas, err := cfc.client.OrganizationQuotas.ListAll(context.Background(), &cfclient.OrganizationQuotaListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting v3 organization quotas: %w", err)
	}

	var allQuotas []CFOrgQuota
	for _, quota := range quotas {
		memoryLimit := unlimitedQuota
		if quota.Apps.TotalMemoryInMB != nil {
			memoryLimit = *quota.Apps.TotalMemoryInMB
		}
		allQuotas = append(allQuotas, CFOrgQuota{
			GUID:        quota.GUID,
			MemoryLimit: memoryLimit,
		})
	}

	return allQuotas, nil
}

func (cfc *CFClient) getCFSidecars(appGUID string) ([]CFSidecar, error) {
	sidecars, err := cfc.client.Sidecars.ListForAppAll(context.Background(), appGUID, &cfclient.SidecarListOptions{ListOptions: cfc.listOptions()})
	if err != nil {
		return nil, fmt.Errorf("Error requesting sidecars for app: %s", err)
	}

	// nil when there are no sidecars: the app parser only emits sidecar tags for a non-nil slice
	var result []CFSidecar
	for _, sidecar := range sidecars {
		result = append(result, CFSidecar{Name: sidecar.Name, GUID: sidecar.GUID})
	}
	return result, nil
}

func (a *CFApplication) setV3AppData(data *resource.App) {
	a.GUID = data.GUID
	a.Name = data.Name
	if data.Relationships.Space.Data != nil {
		a.SpaceGUID = data.Relationships.Space.Data.GUID
	}
	switch lifecycle := data.Lifecycle.Data.(type) {
	case *resource.BuildpackLifecycle:
		a.Buildpacks = lifecycle.Buildpacks
	case *resource.CNBLifecycle:
		a.Buildpacks = lifecycle.Buildpacks
	}
	a.Annotations = map[string]string{}
	a.Labels = map[string]string{}
	if data.Metadata != nil {
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
		mergeMetadata(a.Labels, data.Metadata.Labels)
	}
}

func (a *CFApplication) setV3ProcessData(data []*resource.Process) {
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

func (a *CFApplication) setV3SpaceData(data *resource.Space) {
	a.SpaceName = data.Name
	if data.Relationships != nil && data.Relationships.Organization != nil && data.Relationships.Organization.Data != nil {
		a.OrgGUID = data.Relationships.Organization.Data.GUID
	}

	// Set space labels and annotations only if they're not overriden per application
	if data.Metadata != nil {
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
		mergeMetadata(a.Labels, data.Metadata.Labels)
	}
}

func (a *CFApplication) setV3OrgData(data *resource.Organization) {
	a.OrgName = data.Name

	// Set org labels and annotations only if they're not overriden per space or application
	if data.Metadata != nil {
		mergeMetadata(a.Annotations, data.Metadata.Annotations)
		mergeMetadata(a.Labels, data.Metadata.Labels)
	}
}

// mergeMetadata adds the non-null entries of metadata that are not already set in dst.
func mergeMetadata(dst map[string]string, metadata map[string]*string) {
	for key, value := range metadata {
		if value == nil {
			continue
		}
		if _, ok := dst[key]; !ok {
			dst[key] = *value
		}
	}
}

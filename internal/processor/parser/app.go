package parser

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/pkg/errors"
)

type appCache struct {
	apps map[string]*App
	lock sync.RWMutex
}

func newAppCache() appCache {
	return appCache{
		apps: make(map[string]*App),
	}
}

// Add inserts or update a new app in the cache, and returns it
func (c *appCache) Add(cfApp cfclient.App) *App {
	c.lock.Lock()
	defer c.lock.Unlock()

	if app := c.apps[cfApp.Guid]; app != nil {
		app.setAppData(cfApp)
	} else {
		app := newApp(cfApp.Guid)
		app.setAppData(cfApp)
		c.apps[cfApp.Guid] = app
	}

	return c.apps[cfApp.Guid]
}

// Delete removes an app from the cache
func (c *appCache) Delete(guid string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.apps, guid)
}

// Get returns a cached app or nil if not found
func (c *appCache) Get(guid string) *App {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.apps[guid]
}

// AppParser is used to parse app metrics
type AppParser struct {
	CFClient     *cfclient.Client
	log          *gosteno.Logger
	AppCache     appCache
	cacheWorkers int
	grabInterval int
	customTags   []string
	stopper      chan bool
}

// NewAppParser create a new AppParser
func NewAppParser(
	cfClient *cfclient.Client,
	cacheWorkers int,
	grabInterval int,
	log *gosteno.Logger,
	customTags []string,
	environment string,
) (*AppParser, error) {

	if cfClient == nil {
		return nil, fmt.Errorf("The CF Client needs to be properly set up to use appmetrics")
	}
	if environment != "" {
		customTags = append(customTags, fmt.Sprintf("%s:%s", "env", environment))
	}
	appMetrics := &AppParser{
		CFClient:     cfClient,
		log:          log,
		AppCache:     newAppCache(),
		cacheWorkers: cacheWorkers,
		grabInterval: grabInterval,
		customTags:   customTags,
		stopper:      make(chan bool, 1),
	}

	appMetrics.warmupCache()
	// start the background loop to keep the cache up to date
	go appMetrics.updateCacheLoop()

	return appMetrics, nil
}

// updateCacheLoop periodically refreshes the entire cache
func (am *AppParser) updateCacheLoop() {
	ticker := time.NewTicker(time.Duration(am.grabInterval) * time.Minute)
	for {
		select {
		case <-ticker.C:
			am.warmupCache()
		case <-am.stopper:
			return
		}
	}
}

func (am *AppParser) warmupCache() {
	am.log.Infof("Warming up cache...")

	apps, err := listApps(am.CFClient, am.cacheWorkers, am.log)
	if err != nil {
		am.log.Errorf("Error warming up cache, couldn't get list of apps: %v", err)
		return
	}

	for _, resolvedApp := range apps {
		am.AppCache.Add(resolvedApp)
	}
	am.log.Infof("Done warming up cache")
}

func (am *AppParser) getAppData(guid string) (*App, error) {
	app := am.AppCache.Get(guid)
	if app != nil {
		// If it exists in the cache, use the cache
		return app, nil
	}

	// Otherwise it's a new app so fetch it via the API
	resolvedApp, err := am.CFClient.AppByGuid(guid)
	if err != nil {
		am.log.Errorf("there was an error grabbing the instance data for app %v: %v", resolvedApp.Guid, err)
		return nil, err
	}

	return am.AppCache.Add(resolvedApp), nil
}

// Parse takes an envelope, and extract app metrics from it
func (am *AppParser) Parse(envelope *events.Envelope) ([]metric.MetricPackage, error) {
	metricsPackages := []metric.MetricPackage{}
	message := envelope.GetContainerMetric()

	guid := message.GetApplicationId()
	app, err := am.getAppData(guid)
	if err != nil || app == nil {
		am.log.Errorf("there was an error grabbing data for app %v: %v", guid, err)
		return metricsPackages, err
	}

	app.lock.Lock()
	defer app.lock.Unlock()

	app.Host = envelope.GetOrigin()

	metricsPackages = app.getMetrics(am.customTags)
	containerMetrics, err := app.parseContainerMetric(message, am.customTags)
	if err != nil {
		return metricsPackages, err
	}
	metricsPackages = append(metricsPackages, containerMetrics...)

	return metricsPackages, nil
}

// listApps is meant to replace the function from the go-cfclient and allow fetching apps in parallel tasks
// The apps returned by this are just missing a reference to the client (unexported property) so don't use the Space() and Summary() methods
func listApps(c *cfclient.Client, numWorkers int, log *gosteno.Logger) ([]cfclient.App, error) {

	// Query the first page to get the total number of pages.
	resp, err := getAppsPage(c, 1)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting apps page 1, skipping cache warmup")
	}
	appResources := resp.Resources

	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Page 1 already fetched
	pages := resp.Pages - 1
	// No need for more workers than pages left
	numWorkers = int(math.Min(float64(numWorkers), float64(pages)))
	if pages > 0 {
		pagesPerWorker := int(math.Ceil(float64(pages) / float64(numWorkers)))

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				// Offset 2 because no page 0 and page 1 already fetched
				pageStart := i*pagesPerWorker + 2
				// Stop at page resp.Pages, which is the last one
				pageEnd := int(math.Min(float64((i+1)*pagesPerWorker+2), float64(resp.Pages)))
				resources := getAppResourcesPageRange(c, pageStart, pageEnd, log)
				mutex.Lock()
				appResources = append(appResources, resources...)
				mutex.Unlock()
			}(i)
		}

		wg.Wait()
	}

	apps := []cfclient.App{}
	for _, app := range appResources {
		// Taken from https://github.com/cloudfoundry-community/go-cfclient/blob/16c98753d3152f9d80d3c121523536858095a3da/apps.go#L643
		app.Entity.Guid = app.Meta.Guid
		app.Entity.CreatedAt = app.Meta.CreatedAt
		app.Entity.UpdatedAt = app.Meta.UpdatedAt
		app.Entity.SpaceData.Entity.Guid = app.Entity.SpaceData.Meta.Guid
		app.Entity.SpaceData.Entity.OrgData.Entity.Guid = app.Entity.SpaceData.Entity.OrgData.Meta.Guid
		apps = append(apps, app.Entity)
	}
	return apps, nil
}

func getAppsPage(c *cfclient.Client, pageNb int) (*cfclient.AppResponse, error) {
	// Taken from https://github.com/cloudfoundry-community/go-cfclient/blob/16c98753d3152f9d80d3c121523536858095a3da/apps.go#L332
	var appResp cfclient.AppResponse
	q := url.Values{}
	q.Set("inline-relations-depth", "2")
	q.Set("results-per-page", "100") // 100 is the max
	q.Set("page", string(pageNb))
	r := c.NewRequest("GET", "/v2/apps?"+q.Encode())
	resp, err := c.DoRequest(r)

	if err != nil {
		return nil, errors.Wrapf(err, "Error requesting apps page %d", pageNb)
	}
	defer resp.Body.Close()
	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading app response for page %d", pageNb)
	}

	err = json.Unmarshal(resBody, &appResp)
	if err != nil {
		return nil, errors.Wrapf(err, "Error unmarshalling app response for page %d", pageNb)
	}

	return &appResp, nil
}

func getAppResourcesPageRange(c *cfclient.Client, pageStart, pageEnd int, log *gosteno.Logger) []cfclient.AppResource {
	appResources := []cfclient.AppResource{}
	for i := pageStart; i < pageEnd; i++ {
		resp, err := getAppsPage(c, i)
		if err != nil {
			log.Error(err.Error())
			continue
		}
		appResources = append(appResources, resp.Resources...)
	}
	return appResources
}

// Stop sends a message on the stopper channel to quit the goroutine refreshing the cache
func (am *AppParser) Stop() {
	am.stopper <- true
}

// App holds all the needed attribute from an app
type App struct {
	Name                   string
	Host                   string
	Buildpack              string
	Command                string
	Diego                  bool
	OrgName                string
	OrgID                  string
	Routes                 []string
	SpaceID                string
	SpaceName              string
	SpaceURL               string
	GUID                   string
	DockerImage            string
	NumberOfInstances      int
	TotalDiskConfigured    int
	TotalMemoryConfigured  int
	TotalDiskProvisioned   int
	TotalMemoryProvisioned int
	Tags                   []string
	lock                   sync.RWMutex
}

func newApp(guid string) *App {
	return &App{
		GUID: guid,
	}
}

func (a *App) getMetrics(customTags []string) []metric.MetricPackage {
	var names = []string{
		"app.disk.configured",
		"app.disk.provisioned",
		"app.memory.configured",
		"app.memory.provisioned",
		"app.instances",
	}

	var ms = []float64{
		float64(a.TotalDiskConfigured),
		float64(a.TotalDiskProvisioned),
		float64(a.TotalMemoryConfigured),
		float64(a.TotalMemoryProvisioned),
		float64(a.NumberOfInstances),
	}

	return a.mkMetrics(names, ms, customTags)
}

func (a *App) parseContainerMetric(message *events.ContainerMetric, customTags []string) ([]metric.MetricPackage, error) {
	var names = []string{
		"app.cpu.pct",
		"app.disk.used",
		"app.disk.quota",
		"app.memory.used",
		"app.memory.quota",
	}
	var ms = []float64{
		float64(message.GetCpuPercentage()),
		float64(message.GetDiskBytes()),
		float64(message.GetDiskBytesQuota()),
		float64(message.GetMemoryBytes()),
		float64(message.GetMemoryBytesQuota()),
	}
	tags := []string{fmt.Sprintf("instance:%v", message.GetInstanceIndex())}
	tags = append(tags, customTags...)

	return a.mkMetrics(names, ms, tags), nil
}

func (a *App) mkMetrics(names []string, ms []float64, moreTags []string) []metric.MetricPackage {
	metricsPackages := []metric.MetricPackage{}
	var host string
	if a.Host != "" {
		host = a.Host
	} else {
		host = a.GUID
	}

	tags := a.getTags()
	tags = append(tags, moreTags...)

	for i, name := range names {
		key := metric.MetricKey{
			Name:     name,
			TagsHash: util.HashTags(tags),
		}
		mVal := metric.MetricValue{
			Tags: tags,
			Host: host,
		}
		p := metric.Point{
			Timestamp: time.Now().Unix(),
			Value:     float64(ms[i]),
		}
		mVal.Points = append(mVal.Points, p)
		metricsPackages = append(metricsPackages, metric.MetricPackage{
			MetricKey:   &key,
			MetricValue: &mVal,
		})
	}

	return metricsPackages
}

func (a *App) getTags() []string {
	if a.Tags != nil && len(a.Tags) > 0 {
		return a.Tags
	}

	a.Tags = a.generateTags()
	return a.Tags
}

func (a *App) generateTags() []string {
	var tags = []string{}
	if a.Name != "" {
		tags = append(tags, fmt.Sprintf("app_name:%v", a.Name))
	}
	if a.Buildpack != "" {
		tags = append(tags, fmt.Sprintf("buildpack:%v", a.Buildpack))
	}
	if a.Command != "" {
		tags = append(tags, fmt.Sprintf("command:%v", a.Command))
	}
	if a.Diego {
		tags = append(tags, fmt.Sprintf("diego"))
	}
	if a.OrgName != "" {
		tags = append(tags, fmt.Sprintf("org_name:%v", a.OrgName))
	}
	if a.OrgID != "" {
		tags = append(tags, fmt.Sprintf("org_id:%v", a.OrgID))
	}
	if a.SpaceName != "" {
		tags = append(tags, fmt.Sprintf("space_name:%v", a.SpaceName))
	}
	if a.SpaceID != "" {
		tags = append(tags, fmt.Sprintf("space_id:%v", a.SpaceID))
	}
	if a.SpaceURL != "" {
		tags = append(tags, fmt.Sprintf("space_url:%v", a.SpaceURL))
	}
	if a.GUID != "" {
		tags = append(tags, fmt.Sprintf("guid:%v", a.GUID))
	}
	if a.DockerImage != "" {
		tags = append(tags, fmt.Sprintf("image:%v", a.DockerImage))
	}

	return tags
}

func (a *App) setAppData(resolvedApp cfclient.App) {
	// See https://apidocs.cloudfoundry.org/9.0.0/apps/retrieve_a_particular_app.html for the description of attributes
	a.Name = resolvedApp.Name
	if resolvedApp.Buildpack != "" {
		a.Buildpack = resolvedApp.Buildpack
	} else if resolvedApp.DetectedBuildpack != "" {
		a.Buildpack = resolvedApp.DetectedBuildpack
	}
	a.Command = resolvedApp.Command
	a.DockerImage = resolvedApp.DockerImage
	a.Diego = resolvedApp.Diego
	a.SpaceID = resolvedApp.SpaceGuid
	a.NumberOfInstances = resolvedApp.Instances

	a.TotalDiskConfigured = resolvedApp.DiskQuota
	a.TotalMemoryConfigured = resolvedApp.Memory
	a.TotalDiskProvisioned = resolvedApp.DiskQuota * a.NumberOfInstances
	a.TotalMemoryProvisioned = resolvedApp.Memory * a.NumberOfInstances

	a.SpaceName = resolvedApp.SpaceData.Entity.Name
	a.OrgName = resolvedApp.SpaceData.Entity.OrgData.Entity.Name
	a.OrgID = resolvedApp.SpaceData.Entity.OrgData.Entity.Guid

	a.Tags = a.generateTags()
}

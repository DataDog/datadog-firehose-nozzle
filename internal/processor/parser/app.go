package parser

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/cloudfoundry/gosteno"
)

type appCache struct {
	apps     map[string]*App
	warmedUp bool
	lock     sync.RWMutex
}

func newAppCache() appCache {
	return appCache{
		apps:     make(map[string]*App),
		warmedUp: false,
	}
}

// Add inserts or update a new app in the cache, and returns it
func (c *appCache) Add(cfApp cloudfoundry.CFApplication) (*App, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if app := c.apps[cfApp.GUID]; app != nil {
		err := app.setAppData(cfApp)
		if err != nil {
			return nil, err
		}
	} else {
		app := newApp(cfApp.GUID)
		err := app.setAppData(cfApp)
		if err != nil {
			return nil, err
		}
		c.apps[cfApp.GUID] = app
	}

	return c.apps[cfApp.GUID], nil
}

// Get returns a cached app or nil if not found
func (c *appCache) Get(guid string) *App {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.apps[guid]
}

// IsWarmedUp returns true if the cache has completed its first warmup cycle
func (c *appCache) IsWarmedUp() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.warmedUp
}

// setWarmedUp signals to the cache that it's ready to be used
func (c *appCache) SetWarmedUp() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.warmedUp = true
}

// AppParser is used to parse app metrics
type AppParser struct {
	cfClient     *cloudfoundry.CFClient
	dcaClient    *cloudfoundry.DCAClient
	log          *gosteno.Logger
	AppCache     appCache
	cacheWorkers int
	grabInterval int
	customTags   []string
	stopper      chan bool
}

// NewAppParser create a new AppParser
func NewAppParser(
	cfClient *cloudfoundry.CFClient,
	dcaClient *cloudfoundry.DCAClient,
	cacheWorkers int,
	grabInterval int,
	log *gosteno.Logger,
	customTags []string,
	environment string,
) (*AppParser, error) {

	if cfClient == nil && dcaClient == nil {
		return nil, fmt.Errorf("At least one CF Client or DCA Client needs to be properly set up to use appmetrics")
	}

	if environment != "" {
		customTags = append(customTags, fmt.Sprintf("%s:%s", "env", environment))
	}
	appMetrics := &AppParser{
		cfClient:     cfClient,
		dcaClient:    dcaClient,
		log:          log,
		AppCache:     newAppCache(),
		cacheWorkers: cacheWorkers,
		grabInterval: grabInterval,
		customTags:   customTags,
		stopper:      make(chan bool, 1),
	}

	// start the background loop to keep the cache up to date
	go appMetrics.updateCacheLoop()

	return appMetrics, nil
}

// updateCacheLoop periodically refreshes the entire cache
func (am *AppParser) updateCacheLoop() {
	// Run first cache warmup
	am.warmupCache()

	// Start a ticker to update the cache at regular intervals with a random jitter of 10 %
	// to distribute the load on CF Cloud Controller when there are lots of nozzle instances
	// IOW, if the grabInterval is 10 minutes, the warmup will start between 9:00 and 9:59
	ticker, jitterWait := util.GetTickerWithJitter(uint32(am.grabInterval*60), 0.1)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			jitterWait()
			am.warmupCache()
		case <-am.stopper:
			return
		}
	}
}

func (am *AppParser) warmupCache() {
	am.log.Infof("Warming up cache...")

	var cfapps []cloudfoundry.CFApplication
	var err error

	if config.NozzleConfig.DCAEnabled && am.dcaClient != nil {
		fmt.Printf("using cluster agent client to warm up cache")
		cfapps, err = am.dcaClient.GetCFApplications()
		if err != nil {
			am.log.Errorf("error warming up cache using DCA client, couldn't get list of apps: %v", err)
			return
		}
	} else if am.cfClient != nil {
		fmt.Printf("using cloud foundry client to warm up cache")
		cfapps, err = am.cfClient.GetApplications()
		if err != nil {
			am.log.Errorf("error warming up cache using CF client, couldn't get list of apps: %v", err)
			return
		}
	} else {
		am.log.Errorf("error warming up cache, both CFClient and DCA Client are not initialized")
	}

	for _, cfapp := range cfapps {
		_, err := am.AppCache.Add(cfapp)
		if err != nil {
			am.log.Errorf("an error occurred when adding app to the cache: %v", err)
			// We intentionally continue adding apps if a single app fails
		}
	}
	if !am.AppCache.IsWarmedUp() {
		am.AppCache.SetWarmedUp()
	}
	am.log.Infof("done warming up cache")
}

func (am *AppParser) getAppData(guid string) (*App, error) {
	app := am.AppCache.Get(guid)
	if app != nil {
		// If it exists in the cache, use the cache
		return app, nil
	}
	// Otherwise it's a new app so fetch it via the API
	cfapp, err := am.cfClient.GetApplication(guid)
	if err != nil {
		am.log.Warnf("error grabbing instance data for app %s (is this a short-lived app?): %v", guid, err)
		return nil, err
	}
	app, err = am.AppCache.Add(*cfapp)
	if err != nil {
		am.log.Errorf("an error occurred when adding app to the cache: %v", err)
	}

	return app, nil
}

// Parse takes an envelope, and extract app metrics from it
func (am *AppParser) Parse(envelope *loggregator_v2.Envelope) ([]metric.MetricPackage, error) {
	metricsPackages := []metric.MetricPackage{}

	if !util.IsContainerMetric(envelope) {
		return metricsPackages, fmt.Errorf("not an app metric")
	}

	message := envelope.GetGauge()

	guid := envelope.GetSourceId()
	if guid == "" {
		am.log.Errorf("there was an error grabbing ApplicationId from message")
		return metricsPackages, nil
	}
	app, err := am.getAppData(guid)
	if err != nil || app == nil {
		// the problem was already logged in getAppData method
		return metricsPackages, err
	}

	app.lock.Lock()
	defer app.lock.Unlock()

	app.Host = parseHost(envelope)

	metricsPackages, err = app.getMetrics(am.customTags)
	if err != nil {
		am.log.Errorf("there was an error parsing metrics: %v", err)
		return metricsPackages, err
	}
	containerMetrics, err := app.parseContainerMetric(message, envelope.GetInstanceId(), am.customTags)
	if err != nil {
		am.log.Errorf("there was an error parsing container metrics: %v", err)
		return metricsPackages, err
	}
	metricsPackages = append(metricsPackages, containerMetrics...)

	return metricsPackages, nil
}

// Stop sends a message on the stopper channel to quit the goroutine refreshing the cache
func (am *AppParser) Stop() {
	am.stopper <- true
}

// App holds all the needed attribute from an app
type App struct {
	Name                   string
	GUID                   string
	SpaceID                string
	SpaceName              string
	SpaceURL               string
	OrgName                string
	OrgID                  string
	Host                   string
	Buildpacks             []string
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

func (a *App) getMetrics(customTags []string) ([]metric.MetricPackage, error) {
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

func (a *App) parseContainerMetric(message *loggregator_v2.Gauge, instanceID string, customTags []string) ([]metric.MetricPackage, error) {
	var names = []string{
		"app.cpu.pct",
		"app.disk.used",
		"app.disk.quota",
		"app.memory.used",
		"app.memory.quota",
	}

	// App.Parse checks that this is container metric, so we're guaranteed that all these tags are present
	var ms = []float64{
		float64(message.GetMetrics()["cpu"].Value),
		float64(message.GetMetrics()["disk"].Value),
		float64(message.GetMetrics()["disk_quota"].Value),
		float64(message.GetMetrics()["memory"].Value),
		float64(message.GetMetrics()["memory_quota"].Value),
	}
	tags := []string{fmt.Sprintf("instance:%v", getContainerInstanceID(message, instanceID))}
	tags = append(tags, customTags...)
	return a.mkMetrics(names, ms, tags)
}

func (a *App) mkMetrics(names []string, ms []float64, moreTags []string) ([]metric.MetricPackage, error) {
	metricsPackages := []metric.MetricPackage{}
	var host string
	if a.Host != "" {
		host = a.Host
	} else {
		host = a.GUID
	}
	tags := make([]string, len(a.Tags))
	copy(tags, a.Tags)
	tags = append(tags, moreTags...)
	// source_id in envelope always matches app.GUID for app metrics
	tags = appendTagIfNotEmpty(tags, "source_id", a.GUID)

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

	return metricsPackages, nil
}

func (a *App) setAppData(cfapp cloudfoundry.CFApplication) error {
	a.Name = cfapp.Name
	a.SpaceID = cfapp.SpaceGUID
	a.SpaceName = cfapp.SpaceName
	a.OrgName = cfapp.OrgName
	a.OrgID = cfapp.OrgGUID
	a.NumberOfInstances = cfapp.Instances
	a.TotalDiskConfigured = cfapp.DiskQuota
	a.TotalMemoryConfigured = cfapp.Memory
	a.TotalDiskProvisioned = cfapp.TotalDiskQuota
	a.TotalMemoryProvisioned = cfapp.TotalMemory
	a.Buildpacks = cfapp.Buildpacks

	var tags = []string{}
	tags = appendTagIfNotEmpty(tags, "app_name", a.Name)
	tags = appendTagIfNotEmpty(tags, "org_name", a.OrgName)
	tags = appendTagIfNotEmpty(tags, "org_id", a.OrgID)
	tags = appendTagIfNotEmpty(tags, "space_name", a.SpaceName)
	tags = appendTagIfNotEmpty(tags, "space_id", a.SpaceID)
	tags = appendTagIfNotEmpty(tags, "guid", a.GUID)
	if len(tags) != 6 {
		return fmt.Errorf("some tags could not be found app_name:%s, "+
			"org_name:%s, org_id:%s, space_name:%s, space_id:%s, guid:%s", a.Name, a.OrgName, a.OrgID, a.SpaceName,
			a.SpaceID, a.GUID)
	}

	if len(a.Buildpacks) > 0 {
		for _, bp := range a.Buildpacks {
			tags = appendTagIfNotEmpty(tags, "buildpack", bp)
		}
	}

	// Append labels and annotations
	tags = appendMetadataTags(tags, cfapp.Annotations, "annotation/")
	tags = appendMetadataTags(tags, cfapp.Labels, "label/")

	a.Tags = tags

	return nil
}

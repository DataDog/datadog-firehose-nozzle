package appmetrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

var clearCacheDuration = 60

type AppMetrics struct {
	CFClient     *cfclient.Client
	log          *gosteno.Logger
	Apps         map[string]*App
	appLock      sync.RWMutex
	grabInterval int
}

func New(
	ccEndpoint string,
	client string,
	clientSecret string,
	insecureSSLSkipVerify bool,
	grabInterval int,
	log *gosteno.Logger,
) (*AppMetrics, error) {

	if ccEndpoint == "" {
		return nil, fmt.Errorf("The CC Endpoint needs to be set in order to set up appmetrics")
	}

	cfg := cfclient.Config{
		ApiAddress:        ccEndpoint,
		ClientID:          client,
		ClientSecret:      clientSecret,
		SkipSslValidation: insecureSSLSkipVerify,
		UserAgent:         "datadog-firehose-nozzle",
	}
	cfClient, err := cfclient.NewClient(&cfg)
	if err != nil {
		return nil, err
	}

	appMetrics := &AppMetrics{
		CFClient:     cfClient,
		log:          log,
		Apps:         make(map[string]*App),
		grabInterval: grabInterval,
	}

	go appMetrics.clearCacheLoop()

	return appMetrics, nil
}

func (am *AppMetrics) clearCacheLoop() {
	// If an app hasn't sent a metric in a while,
	// assume that it's either been taken down or
	// that the loggregator is routing it to a different nozzle and remove it from the cache
	ticker := time.NewTicker(time.Duration(clearCacheDuration) * time.Minute)
	for {
		select {
		case <-ticker.C:
			var toRemove = []string{}
			var tenMinutesAgo = (time.Now().Add(-time.Duration(clearCacheDuration) * time.Minute)).Unix()
			am.appLock.Lock()
			for guid, app := range am.Apps {
				app.lock.RLock()
				if app.updated < tenMinutesAgo {
					toRemove = append(toRemove, guid)
				}
				app.lock.RUnlock()
			}
			for _, guid := range toRemove {
				delete(am.Apps, guid)
			}
			am.appLock.Unlock()
		}
	}
}

func (am *AppMetrics) getAppData(guid string) (*App, error) {
	am.appLock.Lock()
	defer am.appLock.Unlock()

	var app *App
	if _, ok := am.Apps[guid]; ok {
		// If it exists in the cache, use the cache
		app = am.Apps[guid]
		timeToGrab := (time.Now().Add(-time.Duration(am.grabInterval) * time.Minute)).Unix()
		if !app.ErrorGrabbing && app.updated > timeToGrab && !app.GrabAgain {
			return app, nil
		}
	} else {
		am.Apps[guid] = newApp(guid)
		app = am.Apps[guid]
	}
	app.lock.Lock()
	defer app.lock.Unlock()

	resolvedApp, err := am.CFClient.AppByGuid(guid)
	if err != nil {
		if app.ErrorGrabbing {
			// If there was a previous error grabbing the app, assume it's been removed and remove it from the cache
			am.log.Errorf("there was an error grabbing the instance data for app %v, removing from cache: %v", resolvedApp.Guid, err)
			delete(am.Apps, guid)
		} else {
			// If there was not, say that there was such an error
			am.log.Errorf("there was an error grabbing the instance data for app %v: %v", resolvedApp.Guid, err)
		}
		// Ensure that ErrorGrabbing is set
		app.ErrorGrabbing = true
		return nil, err
	}

	app.ErrorGrabbing = false
	app.GrabAgain = false
	app.updated = time.Now().Unix()

	if resolvedApp.Name != "" {
		app.Name = resolvedApp.Name

	} else if app.Name == "" {
		app.GrabAgain = true
	}
	if app.Name == "" {
		am.log.Infof("App %v has no name", guid)
	}
	if resolvedApp.Buildpack != "" {
		app.Buildpack = resolvedApp.Buildpack
	} else if app.Buildpack == "" {
		app.GrabAgain = true
	}
	if resolvedApp.Command != "" {
		app.Command = resolvedApp.Command
	} else if app.Command == "" {
		app.GrabAgain = true
	}
	if resolvedApp.DockerImage != "" {
		app.DockerImage = resolvedApp.DockerImage

	} else if app.DockerImage == "" {
		app.GrabAgain = true
	}
	if resolvedApp.Diego {
		app.Diego = resolvedApp.Diego
	} else if app.Diego {
		app.GrabAgain = true
	}

	resolvedInstances, err := am.CFClient.GetAppInstances(guid)
	if err == nil {
		app.Instances = make(map[string]Instance)
		app.NumberOfInstances = len(resolvedInstances)
		for i, inst := range resolvedInstances {
			app.Instances[i] = Instance{
				InstanceIndex: i,
				State:         inst.State,
			}
		}
	} else {
		am.log.Errorf("there was an error grabbing the instance data for app %v: %v", resolvedApp.Guid, err)
	}

	if resolvedApp.DiskQuota != 0 {
		app.TotalDiskConfigured = resolvedApp.DiskQuota
	}
	if resolvedApp.Memory != 0 {
		app.TotalMemoryConfigured = resolvedApp.Memory
	}
	app.TotalDiskProvisioned = resolvedApp.DiskQuota * app.NumberOfInstances
	app.TotalMemoryProvisioned = resolvedApp.Memory * app.NumberOfInstances

	space, err := resolvedApp.Space()
	if err == nil {
		if space.Name != "" {
			app.SpaceName = space.Name
		} else if app.SpaceName == "" {
			app.GrabAgain = true
		}
		if space.Guid != "" {
			app.SpaceID = space.Guid
		} else if app.SpaceID == "" {
			app.GrabAgain = true
		}
		org, e := space.Org()
		if e == nil {
			if org.Name != "" {
				app.OrgName = org.Name
			} else if app.OrgName == "" {
				app.GrabAgain = true
			}
			if org.Guid != "" {
				app.OrgID = org.Guid
			} else if app.OrgID == "" {
				app.GrabAgain = true
			}
		} else {
			am.log.Errorf("there was an error grabbing the space data for app %v in space %v: %v", resolvedApp.Guid, space.Guid, e)
		}
	} else {
		am.log.Errorf("there was an error grabbing the space data for app %v: %v", resolvedApp.Guid, err)
	}

	app.generateTags()
	return app, nil
}

func (am *AppMetrics) ParseAppMetric(envelope *events.Envelope) ([]metrics.MetricPackage, error) {
	metricsPackages := []metrics.MetricPackage{}
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

	metricsPackages = app.getMetrics()
	containerMetrics, err := app.parseContainerMetric(message)
	if err != nil {
		return metricsPackages, err
	}
	metricsPackages = append(metricsPackages, containerMetrics...)

	return metricsPackages, nil
}

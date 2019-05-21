package appmetrics

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
	bolt "github.com/coreos/bbolt"
)

var clearCacheDuration = 60

type AppMetrics struct {
	CFClient     *cfclient.Client
	log          *gosteno.Logger
	Apps         map[string]*App
	appLock      sync.RWMutex
	db           *bolt.DB
	grabInterval int
	customTags   []string
	appBucket    []byte
}

func New(
	cfClient *cfclient.Client,
	grabInterval int,
	log *gosteno.Logger,
	customTags []string,
	db *bolt.DB,
	environment string,
) (*AppMetrics, error) {

	if cfClient == nil {
		return nil, fmt.Errorf("The CF Client needs to be properly set up to use appmetrics")
	}
	if environment != "" {
		customTags = append(customTags, fmt.Sprintf("%s:%s", "env", environment))
	}
	appMetrics := &AppMetrics{
		CFClient:     cfClient,
		log:          log,
		Apps:         make(map[string]*App),
		grabInterval: grabInterval,
		customTags:   customTags,
		appBucket:    []byte("CloudFoundryApps"),
		db:           db,
	}

	// create the cache db or grab the app cache from it
	appMetrics.reloadCache()
	// start the background loop to keep the cache up to date
	go appMetrics.updateCacheLoop()

	return appMetrics, nil
}

func (am *AppMetrics) updateCacheLoop() {
	// If an app hasn't sent a metric in a while,
	// assume that it's either been taken down or
	// that the loggregator is routing it to a different nozzle and remove it from the cache
	ticker := time.NewTicker(time.Duration(clearCacheDuration) * time.Minute)
	for {
		select {
		case <-ticker.C:
			var toRemove = []string{}
			var oneHourAgo = (time.Now().Add(-time.Duration(clearCacheDuration) * time.Minute)).Unix()
			am.appLock.Lock()
			updatedApps := make(map[string][]byte)
			for guid, app := range am.Apps {
				app.lock.RLock()
				if app.updated < oneHourAgo {
					toRemove = append(toRemove, guid)
				} else {
					jsonApp, err := json.Marshal(app)
					if err != nil {
						am.log.Infof("Error marshalling app for database: %v", err)
					}
					updatedApps[guid] = jsonApp
				}
				app.lock.RUnlock()
			}
			for _, guid := range toRemove {
				delete(am.Apps, guid)
			}
			am.appLock.Unlock()

			// update the database after closing the app map
			// since this won't affect the app map, no need to continue touching it
			am.db.Batch(func(tx *bolt.Tx) error {
				b, err := tx.CreateBucketIfNotExists(am.appBucket)
				if err != nil {
					return fmt.Errorf("create bucket: %s", err)
				}
				// delete removed apps
				for _, guid := range toRemove {
					b.Delete([]byte(guid))
				}
				// update modified apps
				for guid, jsonApp := range updatedApps {
					b.Put([]byte(guid), jsonApp)
				}
				return nil
			})
		}
	}
}

func (am *AppMetrics) reloadCache() error {
	err := am.db.Batch(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(am.appBucket)
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		if b == nil {
			return fmt.Errorf("bucket not created")
		}

		am.appLock.Lock()
		defer am.appLock.Unlock()

		return b.ForEach(func(k []byte, v []byte) error {
			guid := string(k)
			var app *App
			err := json.Unmarshal(v, app)
			if err != nil {
				return err
			}
			am.Apps[guid] = app

			return nil
		})
	})

	return err
}

func (am *AppMetrics) getAppData(guid string) (*App, error) {
	am.appLock.Lock()
	defer am.appLock.Unlock()

	var app *App
	if _, ok := am.Apps[guid]; ok {
		// If it exists in the cache, use the cache
		app = am.Apps[guid]
		timeToGrab := (time.Now().Add(-time.Duration(am.grabInterval) * time.Minute)).Unix()
		if !app.ErrorGrabbing && app.updated > timeToGrab {
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
	app.updated = time.Now().Unix()

	// See https://apidocs.cloudfoundry.org/9.0.0/apps/retrieve_a_particular_app.html for the description of attributes
	app.Name = resolvedApp.Name
	if app.Name == "" {
		am.log.Infof("App %v has no name", guid)
	}
	if resolvedApp.Buildpack != "" {
		app.Buildpack = resolvedApp.Buildpack
	} else if resolvedApp.DetectedBuildpack != "" {
		app.Buildpack = resolvedApp.DetectedBuildpack
	}
	app.Command = resolvedApp.Command
	app.DockerImage = resolvedApp.DockerImage
	app.Diego = resolvedApp.Diego
	app.SpaceID = resolvedApp.SpaceGuid

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

	app.TotalDiskConfigured = resolvedApp.DiskQuota
	app.TotalMemoryConfigured = resolvedApp.Memory
	app.TotalDiskProvisioned = resolvedApp.DiskQuota * app.NumberOfInstances
	app.TotalMemoryProvisioned = resolvedApp.Memory * app.NumberOfInstances

	space, err := resolvedApp.Space()
	if err == nil {
		app.SpaceName = space.Name
		org, e := space.Org()
		if e == nil {
			app.OrgName = org.Name
			app.OrgID = org.Guid
		} else {
			app.ErrorGrabbing = true
			am.log.Errorf("there was an error grabbing the org data for app %v in space %v: %v", resolvedApp.Guid, space.Guid, e)
		}
	} else {
		app.ErrorGrabbing = true
		am.log.Errorf("there was an error grabbing the space data for app %v: %v", resolvedApp.Guid, err)
	}

	app.Tags = app.generateTags()
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

	metricsPackages = app.getMetrics(am.customTags)
	containerMetrics, err := app.parseContainerMetric(message, am.customTags)
	if err != nil {
		return metricsPackages, err
	}
	metricsPackages = append(metricsPackages, containerMetrics...)

	return metricsPackages, nil
}

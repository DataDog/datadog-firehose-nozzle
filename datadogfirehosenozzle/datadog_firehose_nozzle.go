package datadogfirehosenozzle

import (
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/localip"
	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/metricProcessor"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/nozzleconfig"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	noaaerrors "github.com/cloudfoundry/noaa/errors"
	"github.com/cloudfoundry/sonde-go/events"
	bolt "github.com/coreos/bbolt"
	"github.com/gorilla/websocket"
)

// DatadogFirehoseNozzle is the struct that holds the state of the nozzle
type DatadogFirehoseNozzle struct {
	config                *nozzleconfig.NozzleConfig
	errs                  <-chan error
	messages              <-chan *events.Envelope
	authTokenFetcher      AuthTokenFetcher
	consumer              *consumer.Consumer
	clients               []*datadogclient.Client
	processor             *metricProcessor.Processor
	cfClient              *cfclient.Client
	processedMetrics      chan []metrics.MetricPackage
	log                   *gosteno.Logger
	db                    *bolt.DB
	appMetrics            bool
	stopper               chan bool
	workersStopper        chan bool
	mapLock               sync.RWMutex
	metricsMap            metrics.MetricsMap // modified by workers & main thread
	totalMessagesReceived uint64             // modified by workers, read by main thread
	slowConsumerAlert     uint64             // modified by workers, read by main thread
	totalMetricsSent      uint64
}

// AuthTokenFetcher is an interface for fetching an auth token from uaa
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

// NewDatadogFirehoseNozzle creates a new nozzle
func NewDatadogFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher, log *gosteno.Logger) *DatadogFirehoseNozzle {
	return &DatadogFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
		metricsMap:       make(metrics.MetricsMap),
		processedMetrics: make(chan []metrics.MetricPackage, 1000),
		log:              log,
		appMetrics:       config.AppMetrics,
		stopper:          make(chan bool),
		workersStopper:   make(chan bool),
	}
}

// Start starts the nozzle
func (d *DatadogFirehoseNozzle) Start() error {
	var authToken string
	var err error
	var db *bolt.DB

	if !d.config.DisableAccessControl {
		authToken = d.authTokenFetcher.FetchAuthToken()
	}

	if d.config.CustomTags == nil {
		d.config.CustomTags = []string{}
	}

	var dbPath = "firehose_nozzle.db"

	if d.config.DBPath != "" {
		dbPath = d.config.DBPath
	}

	db, err = bolt.Open(dbPath, 0666, &bolt.Options{
		ReadOnly: false,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	d.db = db

	d.log.Info("Starting DataDog Firehose Nozzle...")
	// Create Datadog client instances
	d.clients, err = d.createClient()
	if err != nil {
		return err
	}
	// Create Cloud Foundry client instance
	d.cfClient = d.createCFClient()
	// Create metric processor instance
	d.processor = d.createProcessor()
	// Start consumer from the Firehose
	err = d.consumeFirehose(authToken)
	if err != nil {
		return err
	}
	// Start multiple workers to parallelize firehose events (event.envelope) transformation into processedMetrics
	// and then grouped into metricsMap
	d.startWorkers()
	// Start infinite loop to periodically submit metrics (metricsMap) to Datadog
	err = d.postToDatadog()

	// Whenever a stop signal is received the infinite loop within postToDatadog is stopped and then we stop all workers
	d.log.Info("DataDog Firehose Nozzle shutting down...")
	d.stopWorkers()

	return err
}

func (d *DatadogFirehoseNozzle) createClient() ([]*datadogclient.Client, error) {
	ipAddress, err := localip.LocalIP()
	if err != nil {
		panic(err)
	}

	var proxy *datadogclient.Proxy
	if d.config.HTTPProxyURL != "" || d.config.HTTPSProxyURL != "" {
		proxy = &datadogclient.Proxy{
			HTTP:    d.config.HTTPProxyURL,
			HTTPS:   d.config.HTTPSProxyURL,
			NoProxy: d.config.NoProxy,
		}
	}

	// Instantiating Datadog primary client
	var ddClients []*datadogclient.Client
	ddClients = append(ddClients, datadogclient.New(
		d.config.DataDogURL,
		d.config.DataDogAPIKey,
		d.config.MetricPrefix,
		d.config.Deployment,
		ipAddress,
		time.Duration(d.config.DataDogTimeoutSeconds)*time.Second,
		time.Duration(d.config.FlushDurationSeconds)*time.Second,
		d.config.FlushMaxBytes,
		d.log,
		d.config.CustomTags,
		proxy,
	))
	// Instantiating Additional Datadog endpoints
	for endpoint, keys := range d.config.DataDogAdditionalEndpoints {
		for keyIndex := range keys {
			ddClients = append(ddClients, datadogclient.New(
				endpoint,
				keys[keyIndex],
				d.config.MetricPrefix,
				d.config.Deployment,
				ipAddress,
				time.Duration(d.config.DataDogTimeoutSeconds)*time.Second,
				time.Duration(d.config.FlushDurationSeconds)*time.Second,
				d.config.FlushMaxBytes,
				d.log,
				d.config.CustomTags,
				proxy,
			))
		}
	}

	return ddClients, nil
}

func (d *DatadogFirehoseNozzle) createCFClient() *cfclient.Client {
	if d.config.CloudControllerEndpoint == "" {
		d.log.Warnf("The Cloud Controller Endpoint needs to be set in order to set up the cf client")
		return nil
	}

	cfg := cfclient.Config{
		ApiAddress:        d.config.CloudControllerEndpoint,
		ClientID:          d.config.Client,
		ClientSecret:      d.config.ClientSecret,
		SkipSslValidation: d.config.InsecureSSLSkipVerify,
		UserAgent:         "datadog-firehose-nozzle",
	}
	cfClient, err := cfclient.NewClient(&cfg)
	if err != nil {
		d.log.Warnf("Encountered an error while setting up the cf client: %v", err)
		return nil
	}

	return cfClient
}

func (d *DatadogFirehoseNozzle) createProcessor() *metricProcessor.Processor {
	processor := metricProcessor.New(d.processedMetrics, d.config.CustomTags, d.config.EnvironmentName)

	if d.appMetrics {
		appMetrics, err := appmetrics.New(
			d.cfClient,
			d.config.GrabInterval,
			d.log,
			d.config.CustomTags,
			d.db,
			d.config.EnvironmentName,
		)
		if err != nil {
			d.appMetrics = false
			d.log.Warnf("error setting up appMetrics, continuing without application metrics: %v", err)
		} else {
			d.log.Debug("setting up app metrics")
			processor.SetAppMetrics(appMetrics)
		}
	}

	return processor
}

func (d *DatadogFirehoseNozzle) consumeFirehose(authToken string) error {
	if d.config.TrafficControllerURL == "" {
		if d.cfClient != nil {
			d.config.TrafficControllerURL = d.cfClient.Endpoint.DopplerEndpoint
		} else {
			return fmt.Errorf("Either the TrafficController URL or the CC URL needs to be set")
		}
	}

	d.consumer = consumer.New(
		d.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: d.config.InsecureSSLSkipVerify},
		nil)
	d.consumer.SetIdleTimeout(time.Duration(d.config.IdleTimeoutSeconds) * time.Second)
	d.messages, d.errs = d.consumer.FilteredFirehose(d.config.FirehoseSubscriptionID, authToken, consumer.Metrics)

	return nil
}

func (d *DatadogFirehoseNozzle) postToDatadog() error {
	ticker := time.NewTicker(time.Duration(d.config.FlushDurationSeconds) * time.Second)
	for {
		select {
		case <-ticker.C:
			d.PostMetrics()
		case err := <-d.errs:
			d.handleError(err)
			return err
		case <-d.stopper:
			return nil
		}
	}
}

// Stop stops the Nozzle
func (d *DatadogFirehoseNozzle) Stop() {
	go func() {
		d.stopper <- true
	}()
}

// PostMetrics posts metrics do to datadog
func (d *DatadogFirehoseNozzle) PostMetrics() {
	d.mapLock.Lock()
	// deep copy the metrics map to pass to PostMetrics so that we can unlock d.metricsMap while posting
	metricsMap := make(metrics.MetricsMap)
	for k, v := range d.metricsMap {
		metricsMap[k] = v
	}
	totalMessagesReceived := d.totalMessagesReceived
	// Reset the map
	d.metricsMap = make(metrics.MetricsMap)
	d.mapLock.Unlock()

	timestamp := time.Now().Unix()
	for _, client := range d.clients {
		// Add internal metrics
		k, v := client.MakeInternalMetric("totalMessagesReceived", totalMessagesReceived, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("totalMetricsSent", d.totalMetricsSent, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("slowConsumerAlert", atomic.LoadUint64(&d.slowConsumerAlert), timestamp)
		metricsMap[k] = v

		err := client.PostMetrics(metricsMap)
		if err != nil {
			d.log.Errorf("Error posting metrics: %s\n\n", err)
		}
	}

	d.totalMetricsSent += uint64(len(metricsMap))
	d.ResetSlowConsumerError()
}

func (d *DatadogFirehoseNozzle) handleError(err error) {
	if retryErr, ok := err.(noaaerrors.RetryError); ok {
		err = retryErr.Err
	}

	switch closeErr := err.(type) {
	case *websocket.CloseError:
		switch closeErr.Code {
		case websocket.CloseNormalClosure:
		// no op
		case websocket.ClosePolicyViolation:
			d.log.Errorf("Error while reading from the firehose: %v", err)
			d.log.Errorf("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.")
			d.AlertSlowConsumerError()
		default:
			d.log.Errorf("Error while reading from the firehose: %v", err)
		}
	default:
		d.log.Errorf("Error while reading from the firehose: %v", err)

	}

	d.log.Infof("Closing connection with traffic controller due to %v", err)
	d.consumer.Close()
	d.PostMetrics()
}

func (d *DatadogFirehoseNozzle) keepMessage(envelope *events.Envelope) bool {
	return d.config.DeploymentFilter == "" || d.config.DeploymentFilter == envelope.GetDeployment()
}

// ResetSlowConsumerError resets the alert
func (d *DatadogFirehoseNozzle) ResetSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 0)
}

// AlertSlowConsumerError sets the slow consumer alert
func (d *DatadogFirehoseNozzle) AlertSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 1)
}

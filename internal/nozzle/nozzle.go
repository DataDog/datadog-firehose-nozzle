package nozzle

import (
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/client/datadog"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/processor"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	noaaerrors "github.com/cloudfoundry/noaa/errors"
	"github.com/cloudfoundry/sonde-go/events"
	bolt "github.com/coreos/bbolt"
	"github.com/gorilla/websocket"
)

// Nozzle is the struct that holds the state of the nozzle
type Nozzle struct {
	config                *config.Config
	errors                <-chan error
	messages              <-chan *events.Envelope
	authTokenFetcher      AuthTokenFetcher
	consumer              *consumer.Consumer
	ddClients             []*datadog.Client
	processor             *processor.Processor
	cfClient              *cfclient.Client
	processedMetrics      chan []metric.MetricPackage
	log                   *gosteno.Logger
	db                    *bolt.DB
	parseAppMetricsEnable bool
	stopper               chan bool
	workersStopper        chan bool
	mapLock               sync.RWMutex
	metricsMap            metric.MetricsMap // modified by workers & main thread
	totalMessagesReceived uint64            // modified by workers, read by main thread
	slowConsumerAlert     uint64            // modified by workers, read by main thread
	totalMetricsSent      uint64
	callbackLock          sync.RWMutex
}

// AuthTokenFetcher is an interface for fetching an auth token from uaa
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

// Nozzle creates a new nozzle
func NewNozzle(config *config.Config, tokenFetcher AuthTokenFetcher, log *gosteno.Logger) *Nozzle {
	return &Nozzle{
		config:                config,
		authTokenFetcher:      tokenFetcher,
		metricsMap:            make(metric.MetricsMap),
		processedMetrics:      make(chan []metric.MetricPackage, 1000),
		log:                   log,
		parseAppMetricsEnable: config.AppMetrics,
		stopper:               make(chan bool),
		workersStopper:        make(chan bool),
	}
}

// Start starts the nozzle
func (d *Nozzle) Start() error {
	// Fetch Authentication Token
	var authToken string
	if !d.config.DisableAccessControl {
		authToken = d.authTokenFetcher.FetchAuthToken()
	}

	// Fetch Custom Tags
	if d.config.CustomTags == nil {
		d.config.CustomTags = []string{}
	}

	// Initialize Bolt DB
	var dbPath = "firehose_nozzle.db"
	if d.config.DBPath != "" {
		dbPath = d.config.DBPath
	}
	var db, err = bolt.Open(dbPath, 0666, &bolt.Options{
		ReadOnly: false,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	d.db = db

	d.log.Info("Starting DataDog Firehose Nozzle...")

	// Initialize Datadog client instances
	d.ddClients, err = datadog.NewClients(d.config, d.log)
	if err != nil {
		return err
	}

	// Initialize Cloud Foundry client instance
	d.cfClient, err = cloudfoundry.NewClient(d.config, d.log)

	// Initialize Firehose processor
	d.processor, d.parseAppMetricsEnable = processor.NewProcessor(
		d.processedMetrics,
		d.config.CustomTags,
		d.config.EnvironmentName,
		d.parseAppMetricsEnable,
		d.cfClient,
		d.config.GrabInterval,
		d.log,
		d.db)

	// Initialize the firehose consumer (with retry enable)
	err = d.startFirehoseConsumer(authToken)
	if err != nil {
		return err
	}

	// Start multiple workers to parallelize firehose events (event.envelope) transformation into processedMetrics
	// and then grouped into metricsMap
	d.startWorkers()

	// Execute infinite loop.
	// This method is blocking until we get error or a stop signal
	err = d.Run()

	// Whenever a stop signal is received the Run methode above will return. The code below will then be executed
	d.log.Info("DataDog Firehose Nozzle shutting down...")
	// Close Firehose Consumer
	d.log.Infof("Closing connection with traffic controller due to %v", err)
	d.consumer.Close()
	// stop processor
	d.stopWorkers()
	// Submit metrics left in cache if any
	d.PostMetrics()

	return err
}

func (n *Nozzle) startFirehoseConsumer(authToken string) error {
	var err error
	// Initialize the firehose consumer (with retry enable)
	n.consumer, err = n.newFirehoseConsumer(authToken)
	if err != nil {
		return err
	}
	// Run the Firehose consumer
	// It consumes messages from the Firehose and push them to n.messages
	n.messages, n.errors = n.consumer.FilteredFirehose(n.config.FirehoseSubscriptionID, authToken, consumer.Metrics)
	return nil
}

func (d *Nozzle) Run() error {
	// Start infinite loop to periodically:
	// - submit metrics to Datadog
	// - handle error
	//   - log error
	//   - break out of the loop if error is not a retry error
	// - stop nozzle
	//   - break out of the loop
	ticker := time.NewTicker(time.Duration(d.config.FlushDurationSeconds) * time.Second)
	for {
		select {
		case <-ticker.C:
			// Submit metrics to Datadog
			d.PostMetrics()
		case error := <-d.errors:
			// Log error message and figure out if we should retry or shutdown
			//retry := d.handleError(error)
			d.handleError(error)
			//if !retry {
			return error
			//}
		case <-d.stopper:
			return nil
		}
	}
}

func (d *Nozzle) newFirehoseConsumer(authToken string) (*consumer.Consumer, error) {
	if d.config.TrafficControllerURL == "" {
		if d.cfClient != nil {
			d.config.TrafficControllerURL = d.cfClient.Endpoint.DopplerEndpoint
		} else {
			return nil, fmt.Errorf("either the TrafficController URL or the CC URL needs to be set")
		}
	}

	c := consumer.New(
		d.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: d.config.InsecureSSLSkipVerify},
		nil)
	c.SetIdleTimeout(time.Duration(d.config.IdleTimeoutSeconds) * time.Second)
	// retry settings
	c.SetMaxRetryCount(1000)
	c.SetMinRetryDelay(500 * time.Millisecond)
	c.SetMaxRetryDelay(5 * time.Minute)

	return c, nil
}

// Stop stops the Nozzle
func (d *Nozzle) Stop() {
	d.stopper <- true
}

// PostMetrics posts metrics do to datadog
func (d *Nozzle) PostMetrics() {
	d.mapLock.Lock()
	// deep copy the metrics map to pass to PostMetrics so that we can unlock d.metricsMap while posting
	metricsMap := make(metric.MetricsMap)
	for k, v := range d.metricsMap {
		metricsMap[k] = v
	}
	totalMessagesReceived := d.totalMessagesReceived
	// Reset the map
	d.metricsMap = make(metric.MetricsMap)
	d.mapLock.Unlock()

	timestamp := time.Now().Unix()
	for _, client := range d.ddClients {
		// Add internal metrics
		k, v := client.MakeInternalMetric("totalMessagesReceived", totalMessagesReceived, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("totalMetricsSent", d.totalMetricsSent, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("slowConsumerAlert", atomic.LoadUint64(&d.slowConsumerAlert), timestamp)
		metricsMap[k] = v

		err := client.PostMetrics(metricsMap)
		//TODO shoudl we have some retry logic here?
		if err != nil {
			d.log.Errorf("Error posting metrics: %s\n\n", err)
		}
	}

	d.totalMetricsSent += uint64(len(metricsMap))
	d.ResetSlowConsumerError()
}

func (d *Nozzle) handleError(err error) bool {
	// If error is a retry error, we log it and let the consumer retry.
	if retryErr, ok := err.(noaaerrors.RetryError); ok {
		d.log.Errorf("Error while reading from the firehose: %v", retryErr.Error())
		d.log.Info("The Firehose consumer hit a retry error, retrying ...")
		//TODO: Why should we alert with a metric? like for `AlertSlowConsumerError`?
		return true
	}

	// If error is ErrMaxRetriesReached then we log it and shutdown the nozzle
	if err.Error() == consumer.ErrMaxRetriesReached.Error() {
		d.log.Info("Too many retries, shutting down...")
		//TODO: Why should send a DD event?
		return false
	}

	// For other errors, we log it and shutdown the nozzle
	switch unknownError := err.(type) {
	case *websocket.CloseError:
		switch unknownError.Code {
		case websocket.CloseNormalClosure:
			// NOTE: errors with `Code` `websocket.CloseNormalClosure` should not happen since `CloseMessage` control
			// on websocket connection can only happen when we close it.
			// Also this type of error is caught by the consumer. The consumer return nil instead of the error.
			// This is so that the consumer stop instead of retrying
			// see github.com/cloudfoundry/noaa/consumer/async.go#listenForMessages
			d.log.Errorf("Unexpected web socket error with CloseNormalClosure code: %v", err)
		case websocket.ClosePolicyViolation:
			d.log.Errorf("Error while reading from the firehose: %v", err)
			d.log.Errorf("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.")
			//TODO: Why should we alert only on ClosePolicyViolation error code?
			d.AlertSlowConsumerError()
		default:
			d.log.Errorf("Error while reading from the firehose: %v", err)
		}
	default:
		//TODO: Should we report error count to DD?
		d.log.Errorf("Error while reading from the firehose: %v", err)
	}

	return false
}

func (d *Nozzle) keepMessage(envelope *events.Envelope) bool {
	return d.config.DeploymentFilter == "" || d.config.DeploymentFilter == envelope.GetDeployment()
}

// ResetSlowConsumerError resets the alert
func (d *Nozzle) ResetSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 0)
}

// AlertSlowConsumerError sets the slow consumer alert
func (d *Nozzle) AlertSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 1)
}

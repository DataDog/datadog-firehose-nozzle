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
	isStopped             bool
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
		isStopped:             true,
	}
}

// Start starts the nozzle
func (n *Nozzle) Start() error {
	n.log.Info("Starting DataDog Firehose Nozzle...")

	if !n.isStopped {
		n.log.Error("Nozzle is already running")
		return nil
	} else {
		n.isStopped = true
	}

	// Fetch Authentication Token
	var authToken string
	if !n.config.DisableAccessControl {
		authToken = n.authTokenFetcher.FetchAuthToken()
	}

	// Fetch Custom Tags
	if n.config.CustomTags == nil {
		n.config.CustomTags = []string{}
	}

	// Initialize Bolt DB
	var dbPath = "firehose_nozzle.db"
	if n.config.DBPath != "" {
		dbPath = n.config.DBPath
	}
	var db, err = bolt.Open(dbPath, 0666, &bolt.Options{
		ReadOnly: false,
	})
	if err != nil {
		return err
	}
	defer db.Close()
	n.db = db

	// Initialize Datadog client instances
	n.ddClients, err = datadog.NewClients(n.config, n.log)
	if err != nil {
		return err
	}

	// Initialize Cloud Foundry client instance
	n.cfClient, err = cloudfoundry.NewClient(n.config, n.log)

	// Initialize Firehose processor
	n.processor, n.parseAppMetricsEnable = processor.NewProcessor(
		n.processedMetrics,
		n.config.CustomTags,
		n.config.EnvironmentName,
		n.parseAppMetricsEnable,
		n.cfClient,
		n.config.GrabInterval,
		n.log,
		n.db)

	// Initialize the firehose consumer (with retry enable)
	err = n.startFirehoseConsumer(authToken)
	if err != nil {
		return err
	}

	// Start multiple workers to parallelize firehose events (event.envelope) transformation into processedMetrics
	// and then grouped into metricsMap
	n.startWorkers()

	// Execute infinite loop.
	// This method is blocking until we get error or a stop signal
	err = n.run()

	// Whenever a stop signal is received the Run methode above will return. The code below will then be executed
	n.log.Info("DataDog Firehose Nozzle shutting down...")
	// Close Firehose Consumer
	n.log.Infof("Closing connection with traffic controller due to %v", err)
	n.consumer.Close()
	// Stop processor
	n.stopWorkers()
	// Submit metrics left in cache if any
	n.postMetrics()

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

func (n *Nozzle) run() error {
	// Start infinite loop to periodically:
	// - submit metrics to Datadog
	// - handle error
	//   - log error
	//   - break out of the loop if error is not a retry error
	// - stop nozzle
	//   - break out of the loop
	ticker := time.NewTicker(time.Duration(n.config.FlushDurationSeconds) * time.Second)
	for {
		select {
		case <-ticker.C:
			// Submit metrics to Datadog
			n.postMetrics()
		case e := <-n.errors:
			// Log error message and figure out if we should retry or shutdown
			retry := n.handleError(e)
			n.handleError(e)
			if !retry {
				n.isStopped = true
				return e
			}
		case <-n.stopper:
			n.isStopped = true
			return nil
		}
	}
}

func (n *Nozzle) newFirehoseConsumer(authToken string) (*consumer.Consumer, error) {
	if n.config.TrafficControllerURL == "" {
		if n.cfClient != nil {
			n.config.TrafficControllerURL = n.cfClient.Endpoint.DopplerEndpoint
		} else {
			return nil, fmt.Errorf("either the TrafficController URL or the CC URL needs to be set")
		}
	}

	c := consumer.New(
		n.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: n.config.InsecureSSLSkipVerify},
		nil)
	c.SetIdleTimeout(time.Duration(n.config.IdleTimeoutSeconds) * time.Second)
	// retry settings
	c.SetMaxRetryCount(5)
	c.SetMinRetryDelay(500 * time.Millisecond)
	c.SetMaxRetryDelay(time.Minute)

	return c, nil
}

// Stop stops the Nozzle
func (n *Nozzle) Stop() {
	if !n.isStopped {
		n.stopper <- true
	}
}

// PostMetrics posts metrics do to datadog
func (n *Nozzle) postMetrics() {
	n.mapLock.Lock()
	// deep copy the metrics map to pass to PostMetrics so that we can unlock n.metricsMap while posting
	metricsMap := make(metric.MetricsMap)
	for k, v := range n.metricsMap {
		metricsMap[k] = v
	}
	totalMessagesReceived := n.totalMessagesReceived
	// Reset the map
	n.metricsMap = make(metric.MetricsMap)
	n.mapLock.Unlock()

	timestamp := time.Now().Unix()
	for _, client := range n.ddClients {
		// Add internal metrics
		k, v := client.MakeInternalMetric("totalMessagesReceived", totalMessagesReceived, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("totalMetricsSent", n.totalMetricsSent, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("slowConsumerAlert", atomic.LoadUint64(&n.slowConsumerAlert), timestamp)
		metricsMap[k] = v

		err := client.PostMetrics(metricsMap)
		// NOTE: We don't need to have a retry logic since we don't return error on failure.
		// However, current metrics may be lost.
		if err != nil {
			n.log.Errorf("Error posting metrics: %s\n\n", err)
		}
	}

	n.totalMetricsSent += uint64(len(metricsMap))
	n.ResetSlowConsumerError()
}

func (n *Nozzle) handleError(err error) bool {
	// If error is a retry error, we log it and let the consumer retry.
	if retryErr, ok := err.(noaaerrors.RetryError); ok {
		n.log.Errorf("Error while reading from the firehose: %v", retryErr.Error())
		n.log.Info("The Firehose consumer hit a retry error, retrying ...")
		//TODO: Why should we alert with a metric? like for `AlertSlowConsumerError`?
		err = retryErr.Err
	}

	// If error is ErrMaxRetriesReached then we log it and shutdown the nozzle
	if err.Error() == consumer.ErrMaxRetriesReached.Error() {
		n.log.Info("Too many retries, shutting down...")
		//TODO: Why should send a DD event?
		return false
	}

	// For other errors, we log it and shutdown the nozzle
	switch e := err.(type) {
	case *websocket.CloseError:
		switch e.Code {
		case websocket.CloseNormalClosure:
			// NOTE: errors with `Code` `websocket.CloseNormalClosure` should not happen since `CloseMessage` control
			// on websocket connection can only happen when we close it.
			// Also this type of error is caught by the consumer. The consumer return nil instead of the error.
			// This is so that the consumer stop instead of retrying
			// see github.com/cloudfoundry/noaa/consumer/async.go#listenForMessages
			n.log.Errorf("Unexpected web socket error with CloseNormalClosure code: %v", err)
		case websocket.ClosePolicyViolation:
			n.log.Errorf("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.")
			//TODO: Why should we alert only on ClosePolicyViolation error code?
			n.AlertSlowConsumerError()
		default:
			n.log.Errorf("Error while reading from the firehose: %v", err)
		}
	default:
		//TODO: Should we report error count to DD?
		n.log.Errorf("Error while reading from the firehose: %v", err)
	}

	_, retry := err.(noaaerrors.RetryError)
	return retry
}

func (n *Nozzle) keepMessage(envelope *events.Envelope) bool {
	return n.config.DeploymentFilter == "" || n.config.DeploymentFilter == envelope.GetDeployment()
}

// ResetSlowConsumerError resets the alert
func (n *Nozzle) ResetSlowConsumerError() {
	atomic.StoreUint64(&n.slowConsumerAlert, 0)
}

// AlertSlowConsumerError sets the slow consumer alert
func (n *Nozzle) AlertSlowConsumerError() {
	atomic.StoreUint64(&n.slowConsumerAlert, 1)
}

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
	errs                  <-chan error
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
	d.cfClient, err = cloudfoundry.New(d.config, d.log)

	// Initialize Firehose processor
	d.processor, d.parseAppMetricsEnable = processor.New(
		d.processedMetrics,
		d.config.CustomTags,
		d.config.EnvironmentName,
		d.parseAppMetricsEnable,
		d.cfClient,
		d.config.GrabInterval,
		d.log,
		d.db)

	// Let's store the last three times when a restart occurs, so that we don't keep trying forever
	var retryHistory = make([]time.Time, 3)
	for {
		// consume messages from the Firehose and push them to d.messages
		d.consumer, err = d.consumeFirehose(authToken)
		if err != nil {
			return err
		}
		d.messages, d.errs = d.consumer.FilteredFirehose(d.config.FirehoseSubscriptionID, authToken, consumer.Metrics)

		// Start multiple workers to parallelize firehose events (event.envelope) transformation into processedMetrics
		// and then grouped into metricsMap
		d.startWorkers()
		// Start infinite loop to periodically submit metrics (metricsMap) to Datadog
		err = d.postToDatadog()

		if !shouldReconnect(err) {
			break
		}

		// memorize the retry timestamp
		retryHistory = append(retryHistory[1:], time.Now())
		if retryHistory[2].Sub(retryHistory[0]) < 5*time.Second {
			// We retried three times quickly, there might be a larger issue
			// Let's break out of the loop and return the error
			d.log.Info("Too many retries, shutting down...")
			break
		}
		d.stopWorkers()
		// Sleep a little to not reconnect to quickly
		time.Sleep(500 * time.Millisecond)
		d.log.Info("Websocket connection lost, reestablishing connection...")
	}

	// Whenever a stop signal is received the infinite loop within postToDatadog is stopped and then we stop all workers
	d.log.Info("DataDog Firehose Nozzle shutting down...")
	d.stopWorkers()

	return err
}

// Define the error cases where we should reattempt a connection
func shouldReconnect(err error) bool {
	if err == nil {
		return false
	}
	switch err.(type) {
	case noaaerrors.RetryError:
		return true
	case noaaerrors.NonRetryError:
		return false
	default:
		return false
	}
}

func (d *Nozzle) consumeFirehose(authToken string) (*consumer.Consumer, error) {
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

	return c, nil
}

func (d *Nozzle) postToDatadog() error {
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
		if err != nil {
			d.log.Errorf("Error posting metrics: %s\n\n", err)
		}
	}

	d.totalMetricsSent += uint64(len(metricsMap))
	d.ResetSlowConsumerError()
}

func (d *Nozzle) handleError(err error) {
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

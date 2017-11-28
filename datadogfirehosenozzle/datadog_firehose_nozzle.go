package datadogfirehosenozzle

import (
	"crypto/tls"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/localip"
	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/metricProcessor"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/nozzleconfig"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	noaaerrors "github.com/cloudfoundry/noaa/errors"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/websocket"
)

type DatadogFirehoseNozzle struct {
	config                *nozzleconfig.NozzleConfig
	errs                  <-chan error
	messages              <-chan *events.Envelope
	authTokenFetcher      AuthTokenFetcher
	consumer              *consumer.Consumer
	client                *datadogclient.Client
	processor             *metricProcessor.Processor
	processedMetrics      chan []metrics.MetricPackage
	log                   *gosteno.Logger
	appMetrics            bool
	stopper               chan bool
	workersStopper        chan bool
	mapLock               sync.RWMutex
	metricsMap            metrics.MetricsMap // modified by workers & main thread
	totalMessagesReceived uint64             // modified by workers, read by main thread
	slowConsumerAlert     uint64             // modified by workers, read by main thread
	totalMetricsSent      uint64
}

type AuthTokenFetcher interface {
	FetchAuthToken() string
}

func NewDatadogFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher, log *gosteno.Logger) *DatadogFirehoseNozzle {
	return &DatadogFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
		metricsMap:       make(metrics.MetricsMap),
		processedMetrics: make(chan []metrics.MetricPackage),
		log:              log,
		appMetrics:       config.AppMetrics,
		stopper:          make(chan bool),
		workersStopper:   make(chan bool),
	}
}

func (d *DatadogFirehoseNozzle) Start() error {
	var authToken string

	if !d.config.DisableAccessControl {
		authToken = d.authTokenFetcher.FetchAuthToken()
	}

	if d.config.CustomTags == nil {
		d.config.CustomTags = []string{}
	}

	d.log.Info("Starting DataDog Firehose Nozzle...")
	d.client = d.createClient()
	d.processor = d.createProcessor()
	d.consumeFirehose(authToken)
	d.startWorkers()
	err := d.postToDatadog()
	d.stopWorkers()
	d.log.Info("DataDog Firehose Nozzle shutting down...")
	return err
}

func (d *DatadogFirehoseNozzle) createClient() *datadogclient.Client {
	ipAddress, err := localip.LocalIP()
	if err != nil {
		panic(err)
	}

	client := datadogclient.New(
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
	)

	return client
}

func (d *DatadogFirehoseNozzle) createProcessor() *metricProcessor.Processor {
	processor := metricProcessor.New(d.processedMetrics, d.config.CustomTags)

	if d.appMetrics {
		appMetrics, err := appmetrics.New(
			d.config.CloudControllerEndpoint,
			d.config.Client,
			d.config.ClientSecret,
			d.config.InsecureSSLSkipVerify,
			d.config.GrabInterval,
			d.log,
			d.config.CustomTags,
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

func (d *DatadogFirehoseNozzle) consumeFirehose(authToken string) {
	d.consumer = consumer.New(
		d.config.TrafficControllerURL,
		&tls.Config{InsecureSkipVerify: d.config.InsecureSSLSkipVerify},
		nil)
	d.consumer.SetIdleTimeout(time.Duration(d.config.IdleTimeoutSeconds) * time.Second)
	d.messages, d.errs = d.consumer.Firehose(d.config.FirehoseSubscriptionID, authToken)
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

func (d *DatadogFirehoseNozzle) Stop() {
	go func() {
		d.stopper <- true
	}()
}

func (d *DatadogFirehoseNozzle) PostMetrics() {
	d.mapLock.Lock()
	// deep copy the metrics map to pass to PostMetrics so that we can unlock d.metricsMap while posting
	metricsMap := make(metrics.MetricsMap)
	for k, v := range d.metricsMap {
		metricsMap[k] = v
	}
	totalMessagesReceived := d.totalMessagesReceived
	d.mapLock.Unlock()

	// Add internal metrics
	k, v := d.client.MakeInternalMetric("totalMessagesReceived", totalMessagesReceived)
	metricsMap[k] = v
	k, v = d.client.MakeInternalMetric("totalMetricsSent", d.totalMetricsSent)
	metricsMap[k] = v
	k, v = d.client.MakeInternalMetric("slowConsumerAlert", atomic.LoadUint64(&d.slowConsumerAlert))
	metricsMap[k] = v

	err := d.client.PostMetrics(metricsMap)
	if err != nil {
		d.log.Errorf("Error posting metrics: %s\n\n", err)
		return
	}

	d.totalMetricsSent += uint64(len(metricsMap))
	d.mapLock.Lock()
	d.metricsMap = make(metrics.MetricsMap)
	d.mapLock.Unlock()
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

func (d *DatadogFirehoseNozzle) ResetSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 0)
}

func (d *DatadogFirehoseNozzle) AlertSlowConsumerError() {
	atomic.StoreUint64(&d.slowConsumerAlert, 1)
}

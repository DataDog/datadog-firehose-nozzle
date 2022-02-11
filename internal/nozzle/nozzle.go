package nozzle

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/datadog"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/orgcollector"
	"github.com/DataDog/datadog-firehose-nozzle/internal/processor"
	"github.com/cloudfoundry/gosteno"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

// Nozzle is the struct that holds the state of the nozzle
type Nozzle struct {
	config                *config.Config
	messages              chan *loggregator_v2.Envelope
	authTokenFetcher      AuthTokenFetcher
	ddClients             []*datadog.Client
	processor             *processor.Processor
	cfClient              *cloudfoundry.CFClient
	dcaClient             *cloudfoundry.DCAClient
	loggregatorClient     *cloudfoundry.LoggregatorClient
	processedMetrics      chan []metric.MetricPackage
	orgCollector          *orgcollector.OrgCollector
	log                   *gosteno.Logger
	parseAppMetricsEnable bool
	stopper               chan bool
	workersStopper        chan bool
	mapLock               sync.RWMutex
	metricsMap            metric.MetricsMap // modified by workers & main thread
	totalMessagesReceived uint64            // modified by workers, read by main thread
	slowConsumerAlert     uint64            // modified by workers, read by main thread
	totalMetricsSent      uint64
	metricsSent           uint64
	metricsDropped        uint64
}

// AuthTokenFetcher is an interface for fetching an auth token from uaa
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

// NewNozzle creates a new nozzle
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
		messages:              make(chan *loggregator_v2.Envelope, 10000),
	}
}

// Start starts the nozzle
func (n *Nozzle) Start() error {
	n.log.Info("Starting DataDog Firehose Nozzle...")

	// Fetch Custom Tags
	if n.config.CustomTags == nil {
		n.config.CustomTags = []string{}
	}

	n.log.Info("Starting DataDog Firehose Nozzle...")

	// Initialize Datadog client instances
	var err error
	n.ddClients, err = datadog.NewClients(n.config, n.log)
	if err != nil {
		return err
	}

	if n.config.DCAEnabled {
		n.dcaClient, err = cloudfoundry.NewDCAClient(n.config, n.log)
		if err != nil {
			n.log.Warnf("Failed to initialize Datadog Cluster Agent client: %s", err.Error())
		}
	} else {
		// Initialize Cloud Foundry client instance
		n.cfClient, err = cloudfoundry.NewClient(n.config, n.log)
		if err != nil {
			n.log.Warnf("Failed to initialize Cloud Foundry client: %s", err.Error())
		}
	}

	// Initialize Firehose processor
	n.processor, n.parseAppMetricsEnable = processor.NewProcessor(
		n.processedMetrics,
		n.config.CustomTags,
		n.config.EnvironmentName,
		n.parseAppMetricsEnable,
		n.cfClient,
		n.dcaClient,
		n.config.NumCacheWorkers,
		n.config.GrabInterval,
		n.log)

	n.orgCollector, err = orgcollector.NewOrgCollector(
		n.config,
		n.processedMetrics,
		n.log,
		n.config.CustomTags,
	)
	if err != nil {
		n.log.Warnf("Failed to initialize Org metrics collector, org metrics will not be available: %s", err.Error())
	}

	// Start the org collector
	if n.orgCollector != nil {
		n.orgCollector.Start()
	}

	// Initialize the firehose consumer (with retry enable)
	err = n.startFirehoseConsumer(n.authTokenFetcher)
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
	n.log.Infof("Closing connection with loggregator gateway due to %v", err)
	n.loggregatorClient.Stop()
	// Stop processor
	n.stopWorkers()
	// Stop orgCollector
	if n.orgCollector != nil {
		n.orgCollector.Stop()
	}
	// Submit metrics left in cache if any
	n.postMetrics()

	return err
}

func (n *Nozzle) startFirehoseConsumer(authTokenFetcher AuthTokenFetcher) error {
	var err error
	n.loggregatorClient, err = cloudfoundry.NewLoggregatorClient(n.config, n.log, authTokenFetcher)
	if err != nil {
		return err
	}
	envelopeStream := n.loggregatorClient.EnvelopeStream()

	go func(messages chan *loggregator_v2.Envelope, es loggregator.EnvelopeStream) {
		// NOTE: errors in the underlying es() function calls are not returned; they're only logged and
		// the logic retries forever.
		for {
			for _, e := range es() {
				messages <- e
			}
		}
	}(n.messages, envelopeStream)
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
		case <-n.stopper:
			return nil
		}
	}
}

// Stop stops the Nozzle
func (n *Nozzle) Stop() {
	// We only push value to the `stopper` channel of the Nozzle.
	// Hence, if the nozzle is running (`run` method)
	n.stopper <- true
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
		k, v := client.MakeInternalMetric("totalMessagesReceived", metric.GAUGE, totalMessagesReceived, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("totalMetricsSent", metric.GAUGE, n.totalMetricsSent, timestamp)
		metricsMap[k] = v
		k, v = client.MakeInternalMetric("slowConsumerAlert", metric.GAUGE, atomic.LoadUint64(&n.slowConsumerAlert), timestamp)
		metricsMap[k] = v

		if n.totalMetricsSent > 0 {
			k, v = client.MakeInternalMetric("metrics.sent", metric.COUNT, n.metricsSent, timestamp)
			metricsMap[k] = v
			k, v = client.MakeInternalMetric("metrics.dropped", metric.COUNT, n.metricsDropped, timestamp)
			metricsMap[k] = v
			n.metricsSent = 0
			n.metricsDropped = 0
		}

		unsentMetrics := client.PostMetrics(metricsMap)
		n.metricsSent += uint64(len(metricsMap)) - unsentMetrics
		n.metricsDropped += unsentMetrics
	}

	n.totalMetricsSent += n.metricsSent
	n.ResetSlowConsumerError()
}

func (n *Nozzle) keepMessage(envelope *loggregator_v2.Envelope) bool {
	deployment, _ := envelope.GetTags()["deployment"]
	return n.config.DeploymentFilter == "" || n.config.DeploymentFilter == deployment
}

// ResetSlowConsumerError resets the alert
func (n *Nozzle) ResetSlowConsumerError() {
	atomic.StoreUint64(&n.slowConsumerAlert, 0)
}

// AlertSlowConsumerError sets the slow consumer alert
func (n *Nozzle) AlertSlowConsumerError() {
	atomic.StoreUint64(&n.slowConsumerAlert, 1)
}

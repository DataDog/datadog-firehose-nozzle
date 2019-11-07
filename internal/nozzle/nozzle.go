package nozzle

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/client/datadog"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/logger"
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
	messageSelectors      []*loggregator_v2.Selector
	rlpGatewayClient      *loggregator.RLPGatewayClient
	logStreamURL          string
	authTokenFetcher      AuthTokenFetcher
	ddClients             []*datadog.Client
	processor             *processor.Processor
	cfClient              *cloudfoundry.CFClient
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
	stopConsumer          func()
}

// AuthTokenFetcher is an interface for fetching an auth token from uaa
type AuthTokenFetcher interface {
	FetchAuthToken() string
}

type rlpGatewayClientDoer struct {
	token  string
	client *http.Client
}

func (d *rlpGatewayClientDoer) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", d.token)
	return d.client.Do(req)
}

func newRLPGatewayClientDoer(token string, insecureSkipVerify bool) *rlpGatewayClientDoer {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		},
	}

	return &rlpGatewayClientDoer{
		token:  token,
		client: client,
	}
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
		messages: make(chan *loggregator_v2.Envelope, 10000),
	}
}

// Start starts the nozzle
func (n *Nozzle) Start() error {
	n.log.Info("Starting DataDog Firehose Nozzle...")

	// Fetch Authentication Token
	var authToken string
	if !n.config.DisableAccessControl {
		authToken = n.authTokenFetcher.FetchAuthToken()
	}

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

	// Initialize Cloud Foundry client instance
	n.cfClient, err = cloudfoundry.NewClient(n.config, n.log)

	// Initialize Firehose processor
	n.processor, n.parseAppMetricsEnable = processor.NewProcessor(
		n.processedMetrics,
		n.config.CustomTags,
		n.config.EnvironmentName,
		n.parseAppMetricsEnable,
		n.cfClient,
		n.config.NumCacheWorkers,
		n.config.GrabInterval,
		n.log)

	n.messageSelectors = []*loggregator_v2.Selector{
		{
			Message: &loggregator_v2.Selector_Counter{
				Counter: &loggregator_v2.CounterSelector{},
			},
		},
		{
			Message: &loggregator_v2.Selector_Gauge{
				Gauge: &loggregator_v2.GaugeSelector{},
			},
		},
	}

	err = n.setLogStreamURL(n.config)
	if err != nil {
		return err
	}

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
	n.log.Infof("Closing connection with loggregator gateway due to %v", err)
	n.stopConsumer()
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

func (n *Nozzle) setLogStreamURL(c *config.Config) error {
	if c.RLPGatewayURL != "" {
		n.logStreamURL = c.RLPGatewayURL
		return nil
	}

	if c.CloudControllerEndpoint == "" {
		return fmt.Errorf("neither cloud controller endpoint nor RLP gateway URL specified, can't determine log stream URL")
	}

	re := regexp.MustCompile("://(api)")
	n.logStreamURL = re.ReplaceAllString(c.CloudControllerEndpoint, "://log-stream")
	return nil
}

func (n *Nozzle) startFirehoseConsumer(authToken string) error {
	logForwarder := *logger.NewRLPLogForwarder(n.log)
	n.rlpGatewayClient = loggregator.NewRLPGatewayClient(
		n.logStreamURL,
		loggregator.WithRLPGatewayClientLogger(log.New(logForwarder, "", log.LstdFlags)),
		loggregator.WithRLPGatewayHTTPClient(
			newRLPGatewayClientDoer(authToken, n.config.InsecureSSLSkipVerify),
		),
	)

	ctx := context.Background()
	ctx, n.stopConsumer = context.WithCancel(context.Background())
	es := n.rlpGatewayClient.Stream(ctx, &loggregator_v2.EgressBatchRequest{
		ShardId:   n.config.FirehoseSubscriptionID,
		Selectors: n.messageSelectors,
	})

	go func(messages chan *loggregator_v2.Envelope, es loggregator.EnvelopeStream) {
		// NOTE: errors in the underlying es() function calls are not returned; they're only logged and
		// the logic retries forever.
		for {
			for _, e := range es() {
				messages <- e
			}
		}
	}(n.messages, es)
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

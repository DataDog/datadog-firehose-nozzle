package datadogfirehosenozzle

import (
	"crypto/tls"
	"time"

	"code.cloudfoundry.org/localip"
	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/nozzleconfig"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/noaa/consumer"
	noaaerrors "github.com/cloudfoundry/noaa/errors"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/websocket"
)

type DatadogFirehoseNozzle struct {
	config           *nozzleconfig.NozzleConfig
	errs             <-chan error
	messages         <-chan *events.Envelope
	authTokenFetcher AuthTokenFetcher
	consumer         *consumer.Consumer
	client           *datadogclient.Client
	log              *gosteno.Logger
	appMetrics       bool
	stopper          chan bool
	workersStopper   chan bool
}

type AuthTokenFetcher interface {
	FetchAuthToken() string
}

func NewDatadogFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher, log *gosteno.Logger) *DatadogFirehoseNozzle {
	return &DatadogFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
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

	d.log.Info("Starting DataDog Firehose Nozzle...")
	d.client = d.createClient(d.config.CustomTags)
	d.consumeFirehose(authToken)
	d.startWorkers()
	err := d.postToDatadog()
	d.stopWorkers()
	d.log.Info("DataDog Firehose Nozzle shutting down...")
	return err
}

func (d *DatadogFirehoseNozzle) createClient(customTags []string) *datadogclient.Client {
	ipAddress, err := localip.LocalIP()
	if err != nil {
		panic(err)
	}

	if d.config.CustomTags == nil {
		d.config.CustomTags = []string{}
	}

	client := datadogclient.New(
		d.config.DataDogURL,
		d.config.DataDogAPIKey,
		d.config.MetricPrefix,
		d.config.Deployment,
		ipAddress,
		time.Duration(d.config.DataDogTimeoutSeconds)*time.Second,
		d.config.FlushMaxBytes,
		d.log,
		d.config.CustomTags,
	)

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
			client.SetAppMetrics(appMetrics)
		}
	}

	return client
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
			d.postMetrics()
		case err := <-d.errs:
			d.handleError(err)
			return err
		case <-d.stopper:
			return nil
		}
	}
}

func (d *DatadogFirehoseNozzle) work() {
	for {
		select {
		case envelope := <-d.messages:
			if !d.keepMessage(envelope) {
				continue
			}
			d.handleMessage(envelope)
			d.client.ProcessMetric(envelope)
		case <-d.workersStopper:
			return
		}
	}
}

func (d *DatadogFirehoseNozzle) startWorkers() {
	for i := 0; i < d.config.NumWorkers; i++ {
		go d.work()
	}
}

func (d *DatadogFirehoseNozzle) stopWorkers() {
	go func() {
		for i := 0; i < d.config.NumWorkers; i++ {
			d.workersStopper <- true
		}
	}()
}

func (d *DatadogFirehoseNozzle) Stop() {
	go func() {
		d.stopper <- true
	}()
}

func (d *DatadogFirehoseNozzle) postMetrics() {
	err := d.client.PostMetrics()
	if err != nil {
		d.log.Errorf("Error posting metrics: %s\n\n", err)
	}
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
			d.client.AlertSlowConsumerError()
		default:
			d.log.Errorf("Error while reading from the firehose: %v", err)
		}
	default:
		d.log.Errorf("Error while reading from the firehose: %v", err)

	}

	d.log.Infof("Closing connection with traffic controller due to %v", err)
	d.consumer.Close()
	d.postMetrics()
}

func (d *DatadogFirehoseNozzle) keepMessage(envelope *events.Envelope) bool {
	return d.config.DeploymentFilter == "" || d.config.DeploymentFilter == envelope.GetDeployment()
}

func (d *DatadogFirehoseNozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		d.log.Infof("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		d.client.AlertSlowConsumerError()
	}
}

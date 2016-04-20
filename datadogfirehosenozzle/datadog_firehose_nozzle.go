package datadogfirehosenozzle

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/datadog-firehose-nozzle/datadogclient"
	"github.com/cloudfoundry-incubator/datadog-firehose-nozzle/nozzleconfig"
	"github.com/cloudfoundry/noaa/consumer"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/websocket"
	"github.com/pivotal-golang/localip"
)

type DatadogFirehoseNozzle struct {
	config           *nozzleconfig.NozzleConfig
	errs             <-chan error
	messages         <-chan *events.Envelope
	authTokenFetcher AuthTokenFetcher
	consumer         *consumer.Consumer
	client           *datadogclient.Client
}

type AuthTokenFetcher interface {
	FetchAuthToken() string
}

func NewDatadogFirehoseNozzle(config *nozzleconfig.NozzleConfig, tokenFetcher AuthTokenFetcher) *DatadogFirehoseNozzle {
	return &DatadogFirehoseNozzle{
		config:           config,
		authTokenFetcher: tokenFetcher,
	}
}

func (d *DatadogFirehoseNozzle) Start() error {
	var authToken string

	if !d.config.DisableAccessControl {
		authToken = d.authTokenFetcher.FetchAuthToken()
	}

	log.Print("Starting DataDog Firehose Nozzle...")
	d.createClient()
	d.consumeFirehose(authToken)
	err := d.postToDatadog()
	log.Print("DataDog Firehose Nozzle shutting down...")
	return err
}

func (d *DatadogFirehoseNozzle) createClient() {
	ipAddress, err := localip.LocalIP()
	if err != nil {
		panic(err)
	}

	d.client = datadogclient.New(d.config.DataDogURL, d.config.DataDogAPIKey, d.config.MetricPrefix, d.config.Deployment, ipAddress)
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
		case envelope := <-d.messages:
			d.handleMessage(envelope)
			d.client.AddMetric(envelope)
		case err := <-d.errs:
			d.handleError(err)
			return err
		}
	}
}

func (d *DatadogFirehoseNozzle) postMetrics() {
	err := d.client.PostMetrics()
	if err != nil {
		log.Printf("FATAL ERROR: %s\n\n", err)
		os.Exit(1)
	}
}

func (d *DatadogFirehoseNozzle) handleError(err error) {
	switch closeErr := err.(type) {
	case *websocket.CloseError:
		switch closeErr.Code {
		case websocket.CloseNormalClosure:
		// no op
		case websocket.ClosePolicyViolation:
			log.Printf("Error while reading from the firehose: %v", err)
			log.Printf("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle.")
			d.client.AlertSlowConsumerError()
		default:
			log.Printf("Error while reading from the firehose: %v", err)
		}
	default:
		log.Printf("Error while reading from the firehose: %v", err)

	}

	log.Printf("Closing connection with traffic controller due to %v", err)
	d.consumer.Close()
	d.postMetrics()
}

func (d *DatadogFirehoseNozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		log.Printf("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		d.client.AlertSlowConsumerError()
	}
}

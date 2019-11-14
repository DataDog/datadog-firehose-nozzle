package cloudfoundry

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/logger"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/cloudfoundry/gosteno"
)

type LoggregatorClient struct {
	RLPGatewayClient *loggregator.RLPGatewayClient
	stopConsumer     context.CancelFunc
	shardId          string
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

func NewLoggregatorClient(cfg *config.Config, lgr *gosteno.Logger, authToken string) (*LoggregatorClient, error) {
	var logStreamURL string
	if cfg.RLPGatewayURL != "" {
		logStreamURL = cfg.RLPGatewayURL
	} else if cfg.CloudControllerEndpoint != "" {
		re := regexp.MustCompile("://(api)")
		logStreamURL = re.ReplaceAllString(cfg.CloudControllerEndpoint, "://log-stream")
	} else {
		return nil, fmt.Errorf("neither CloudControllerEndpoint nor RLPGatewayURL specified, can't determine log stream URL")
	}

	logForwarder := *logger.NewRLPLogForwarder(lgr)
	return &LoggregatorClient{
		RLPGatewayClient: loggregator.NewRLPGatewayClient(
			logStreamURL,
			loggregator.WithRLPGatewayClientLogger(log.New(logForwarder, "", log.LstdFlags)),
			loggregator.WithRLPGatewayHTTPClient(
				newRLPGatewayClientDoer(authToken, cfg.InsecureSSLSkipVerify),
			),
		),
		shardId:      cfg.FirehoseSubscriptionID,
		stopConsumer: nil,
	}, nil
}

func (l *LoggregatorClient) EnvelopeStream() loggregator.EnvelopeStream {
	ctx := context.Background()
	ctx, l.stopConsumer = context.WithCancel(context.Background())
	es := l.RLPGatewayClient.Stream(
		ctx,
		&loggregator_v2.EgressBatchRequest{
			ShardId: l.shardId,
			Selectors: []*loggregator_v2.Selector{
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
			},
		},
	)

	return es
}

func (l *LoggregatorClient) Stop() {
	if l.stopConsumer != nil {
		l.stopConsumer()
	}
}

package datadogclient

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"io/ioutil"

	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

const DefaultAPIURL = "https://app.datadoghq.com/api/v1"

type Client struct {
	apiURL                string
	apiKey                string
	metrics               chan<- metrics.MetricPackage
	mLock                 sync.RWMutex
	prefix                string
	deployment            string
	ip                    string
	tagsHash              string
	totalMessagesReceived uint64
	totalMetricsSent      uint64
	slowConsumerAlert     uint64
	httpClient            *http.Client
	maxPostBytes          uint32
	log                   *gosteno.Logger
	formatter             Formatter
	appMetrics            *appmetrics.AppMetrics
}

type Payload struct {
	Series []metrics.Series `json:"series"`
}

func New(
	apiURL string,
	apiKey string,
	metrics chan<- metrics.MetricPackage,
	prefix string,
	deployment string,
	ip string,
	writeTimeout time.Duration,
	maxPostBytes uint32,
	log *gosteno.Logger,
) *Client {
	ourTags := []string{
		"deployment:" + deployment,
		"ip:" + ip,
	}

	httpClient := &http.Client{
		Timeout: writeTimeout,
	}

	return &Client{
		apiURL:       apiURL,
		apiKey:       apiKey,
		metrics:      metrics,
		prefix:       prefix,
		deployment:   deployment,
		ip:           ip,
		log:          log,
		tagsHash:     utils.HashTags(ourTags),
		httpClient:   httpClient,
		maxPostBytes: maxPostBytes,
		formatter: Formatter{
			log: log,
		},
	}
}

func (c *Client) AlertSlowConsumerError() {
	c.mLock.Lock()
	c.slowConsumerAlert = uint64(1)
	c.mLock.Unlock()
}

func (c *Client) SetAppMetrics(appMetrics *appmetrics.AppMetrics) {
	c.appMetrics = appMetrics
}

func (c *Client) ProcessMetric(envelope *events.Envelope) {
	c.mLock.Lock()
	c.totalMessagesReceived++
	c.mLock.Unlock()

	var err error
	var metricsPackages []metrics.MetricPackage

	metricsPackages, err = c.ParseInfraMetric(envelope)
	if err == nil {
		for _, m := range metricsPackages {
			c.metrics <- m
		}
		// it can only be one or the other
		return
	}

	metricsPackages, err = c.ParseAppMetric(envelope)
	if err == nil {
		for _, m := range metricsPackages {
			c.metrics <- m
		}
	}
}

func (c *Client) PostMetrics(metricPoints metrics.MetricsMap) error {
	c.populateInternalMetrics(metricPoints)

	numMetrics := len(metricPoints)
	c.log.Infof("Posting %d metrics", numMetrics)

	seriesBytes := c.formatter.Format(c.prefix, c.maxPostBytes, metricPoints)

	c.totalMetricsSent += uint64(len(metricPoints))
	c.mLock.RLock()
	c.slowConsumerAlert = uint64(0) // Modified by AlertSlowConsumerError
	c.mLock.RUnlock()

	for _, data := range seriesBytes {
		if uint32(len(data)) > c.maxPostBytes {
			c.log.Infof("Throwing out metric that exceeds %d bytes", c.maxPostBytes)
			continue
		}

		if err := c.postMetrics(data); err != nil {
			// Retry the request once
			if err := c.postMetrics(data); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Client) postMetrics(seriesBytes []byte) error {
	url := c.seriesURL()
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(seriesBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("failed to read body")
		}
		return fmt.Errorf("datadog request returned HTTP response: %s\nResponse Body: %s", resp.Status, body)
	}

	return nil
}

func (c *Client) seriesURL() string {
	url := fmt.Sprintf("%s?api_key=%s", c.apiURL, c.apiKey)
	return url
}

func (c *Client) populateInternalMetrics(metricPoints metrics.MetricsMap) {
	c.mLock.RLock()
	totalMessagesReceived := c.totalMessagesReceived // Modified by ProcessMetric
	slowConsumerAlert := c.slowConsumerAlert         // Modified by AlertSlowConsumerError
	c.mLock.RUnlock()

	c.addInternalMetric("totalMetricsSent", c.totalMetricsSent, metricPoints)
	c.addInternalMetric("totalMessagesReceived", totalMessagesReceived, metricPoints)
	c.addInternalMetric("slowConsumerAlert", slowConsumerAlert, metricPoints)
}

func (c *Client) addInternalMetric(name string, value uint64, metricPoints metrics.MetricsMap) {
	key := metrics.MetricKey{
		Name:     name,
		TagsHash: c.tagsHash,
	}

	point := metrics.Point{
		Timestamp: time.Now().Unix(),
		Value:     float64(value),
	}

	mValue := metrics.MetricValue{
		Tags: []string{
			fmt.Sprintf("ip:%s", c.ip),
			fmt.Sprintf("deployment:%s", c.deployment),
		},
		Points: []metrics.Point{point},
	}

	metricPoints[key] = mValue
}

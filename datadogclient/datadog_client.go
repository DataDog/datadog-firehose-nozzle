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
	metrics               chan Metric
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

type Metric struct {
	key metrics.MetricKey
	val metrics.MetricValue
}

type MetricMap map[metrics.MetricKey]metrics.MetricValue

func (m MetricMap) add(metric Metric) {
	value, exists := m[metric.key]
	if exists {
		value.Points = append(value.Points, metric.val.Points...)
	} else {
		value = metric.val
	}
	m[metric.key] = value
}

func New(
	apiURL string,
	apiKey string,
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
		metrics:      make(chan Metric),
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
			c.metrics <- Metric{*m.MetricKey, *m.MetricValue}
		}
		// it can only be one or the other
		return
	}

	metricsPackages, err = c.ParseAppMetric(envelope)
	if err == nil {
		for _, m := range metricsPackages {
			c.metrics <- Metric{*m.MetricKey, *m.MetricValue}
		}
	}
}

func (c *Client) PostMetrics() error {
	metricPoints := c.aggregateMetrics()

	numMetrics := len(metricPoints)
	c.log.Infof("Posting %d metrics", numMetrics)

	seriesBytes := c.formatter.Format(c.prefix, c.maxPostBytes, metricPoints)

	c.totalMetricsSent += uint64(len(metricPoints))

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

func (c *Client) aggregateMetrics() MetricMap {
	metricPoints := make(MetricMap)
	timeout := make(chan bool)
	go func() {
		time.Sleep(1 * time.Second)
		timeout <- true
	}()

	// Add new metrics
loop:
	for {
		select {
		case <-timeout:
			break loop
		case metric := <-c.metrics:
			metricPoints.add(metric)
		default:
			break loop
		}
	}

	// Add internal metrics
	c.mLock.RLock()
	totalMessagesReceived := c.totalMessagesReceived
	totalMetricsSent := c.totalMetricsSent
	slowConsumerAlert := c.slowConsumerAlert
	c.slowConsumerAlert = uint64(0)
	c.mLock.RUnlock()

	metricPoints.add(c.makeInternalMetric("totalMessagesReceived", totalMessagesReceived))
	metricPoints.add(c.makeInternalMetric("totalMetricsSent", totalMetricsSent))
	metricPoints.add(c.makeInternalMetric("slowConsumerAlert", slowConsumerAlert))

	return metricPoints
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

func (c *Client) makeInternalMetric(name string, value uint64) Metric {
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

	return Metric{key, mValue}
}

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
	metricPoints          map[metrics.MetricKey]metrics.MetricValue
	mLock                 sync.RWMutex
	prefix                string
	deployment            string
	ip                    string
	customTags            []string
	totalMessagesReceived uint64
	totalMetricsSent      uint64
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
	prefix string,
	deployment string,
	ip string,
	writeTimeout time.Duration,
	maxPostBytes uint32,
	log *gosteno.Logger,
	customTags []string,
) *Client {
	httpClient := &http.Client{
		Timeout: writeTimeout,
	}

	return &Client{
		apiURL:       apiURL,
		apiKey:       apiKey,
		metricPoints: make(map[metrics.MetricKey]metrics.MetricValue),
		prefix:       prefix,
		deployment:   deployment,
		ip:           ip,
		log:          log,
		customTags:   customTags,
		httpClient:   httpClient,
		maxPostBytes: maxPostBytes,
		formatter: Formatter{
			log: log,
		},
	}
}

func (c *Client) AlertSlowConsumerError() {
	c.addInternalMetric("slowConsumerAlert", uint64(1))
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
			c.AddMetric(*m.MetricKey, *m.MetricValue)
		}
		// it can only be one or the other
		return
	}

	metricsPackages, err = c.ParseAppMetric(envelope)
	if err == nil {
		for _, m := range metricsPackages {
			c.AddMetric(*m.MetricKey, *m.MetricValue)
		}
	}
}

func (c *Client) AddMetric(key metrics.MetricKey, value metrics.MetricValue) {
	c.mLock.Lock()
	defer c.mLock.Unlock()

	mVal, ok := c.metricPoints[key]
	if !ok {
		mVal = value
	} else {
		mVal.Points = append(mVal.Points, value.Points...)
	}

	c.metricPoints[key] = mVal
}

func (c *Client) PostMetrics() error {
	c.populateInternalMetrics()

	c.mLock.Lock()
	metricPoints := c.metricPoints
	c.metricPoints = make(map[metrics.MetricKey]metrics.MetricValue)

	numMetrics := len(metricPoints)
	c.log.Infof("Posting %d metrics", numMetrics)

	seriesBytes := c.formatter.Format(c.prefix, c.maxPostBytes, metricPoints)

	c.totalMetricsSent += uint64(len(metricPoints))
	c.mLock.Unlock()

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

func (c *Client) populateInternalMetrics() {
	c.mLock.RLock()
	totalMessagesReceived := c.totalMessagesReceived
	totalMetricsSent := c.totalMetricsSent
	c.mLock.RUnlock()

	c.addInternalMetric("totalMessagesReceived", totalMessagesReceived)
	c.addInternalMetric("totalMetricsSent", totalMetricsSent)

	if !c.containsSlowConsumerAlert() {
		c.addInternalMetric("slowConsumerAlert", uint64(0))
	}
}

func (c *Client) containsSlowConsumerAlert() bool {
	tags := []string{
		fmt.Sprintf("deployment:%s", c.deployment),
		fmt.Sprintf("ip:%s", c.ip),
	}
	tags = append(tags, c.customTags...)

	key := metrics.MetricKey{
		Name:     "slowConsumerAlert",
		TagsHash: utils.HashTags(tags),
	}

	_, ok := c.metricPoints[key]
	return ok
}

func (c *Client) addInternalMetric(name string, value uint64) {
	point := metrics.Point{
		Timestamp: time.Now().Unix(),
		Value:     float64(value),
	}

	tags := []string{
		fmt.Sprintf("deployment:%s", c.deployment),
		fmt.Sprintf("ip:%s", c.ip),
	}
	tags = append(tags, c.customTags...)

	key := metrics.MetricKey{
		Name:     name,
		TagsHash: utils.HashTags(tags),
	}

	mValue := metrics.MetricValue{
		Tags:   tags,
		Points: []metrics.Point{point},
	}

	c.mLock.Lock()
	c.metricPoints[key] = mValue
	c.mLock.Unlock()
}

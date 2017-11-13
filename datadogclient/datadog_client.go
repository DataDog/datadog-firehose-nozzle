package datadogclient

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"io/ioutil"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/gosteno"
)

const DefaultAPIURL = "https://app.datadoghq.com/api/v1"

type Client struct {
	apiURL       string
	apiKey       string
	prefix       string
	deployment   string
	ip           string
	tagsHash     string
	httpClient   *http.Client
	maxPostBytes uint32
	log          *gosteno.Logger
	formatter    Formatter
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

func (c *Client) PostMetrics(metrics metrics.MetricsMap) error {
	c.log.Infof("Posting %d metrics", len(metrics))

	seriesBytes := c.formatter.Format(c.prefix, c.maxPostBytes, metrics)
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

func (c *Client) MakeInternalMetric(name string, value uint64) (metrics.MetricKey, metrics.MetricValue) {
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

	return key, mValue
}

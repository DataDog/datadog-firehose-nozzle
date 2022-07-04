package datadog

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"io/ioutil"

	"code.cloudfoundry.org/localip"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/logs"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/cloudfoundry/gosteno"
	"github.com/hashicorp/go-retryablehttp"
)

type Client struct {
	apiURL       string
	logIntakeURL string
	apiKey       string
	prefix       string
	deployment   string
	ip           string
	customTags   []string
	httpClient   *retryablehttp.Client
	maxPostBytes uint32
	log          *gosteno.Logger
	formatter    Formatter
}

type Payload struct {
	Series []metric.Series `json:"series"`
}

type Proxy struct {
	HTTP    string
	HTTPS   string
	NoProxy []string
}

func New(
	apiURL string,
	logIntakeURL string,
	apiKey string,
	prefix string,
	deployment string,
	ip string,
	writeTimeout time.Duration,
	flushDuration time.Duration,
	maxPostBytes uint32,
	logger *gosteno.Logger,
	customTags []string,
	proxy *Proxy,
) *Client {
	httpClient := retryablehttp.NewClient()
	httpClient.HTTPClient = &http.Client{
		Timeout: writeTimeout,
	}

	// Add a proxy, if it was configured
	if proxy != nil {
		httpClient.HTTPClient.Transport = &http.Transport{
			Proxy: GetProxyTransportFunc(proxy, logger),
		}
	}

	// Set reasonable retry parameters
	// Total time for retry should be <= flushDuration & each retry multiplies the wait time by 2
	httpClient.RetryWaitMin = flushDuration / 7
	httpClient.RetryWaitMax = flushDuration / 2
	httpClient.RetryMax = 3

	// Discard the http client's log and attach our hook for logging request retry attempts
	buffer := new(bytes.Buffer)
	httpClient.Logger = log.New(buffer, "", log.Lshortfile)
	httpClient.RequestLogHook = func(l retryablehttp.Logger, req *http.Request, attemptNum int) {
		if attemptNum == 0 {
			return
		}
		retriesLeft := httpClient.RetryMax - attemptNum
		timeToWait := httpClient.Backoff(httpClient.RetryWaitMin, httpClient.RetryWaitMax, attemptNum-1, nil)
		msg := fmt.Sprintf("Error: %s %s request failed. Wait before retrying: %s (%v left)", req.Method, req.URL, timeToWait, retriesLeft)
		logger.Debug(msg)
	}

	return &Client{
		apiURL:       apiURL,
		logIntakeURL: logIntakeURL,
		apiKey:       apiKey,
		prefix:       prefix,
		deployment:   deployment,
		ip:           ip,
		log:          logger,
		customTags:   customTags,
		httpClient:   httpClient,
		maxPostBytes: maxPostBytes,
		formatter: Formatter{
			log: logger,
		},
	}
}

func NewClients(config *config.Config, log *gosteno.Logger) ([]*Client, error) {
	ipAddress, err := localip.LocalIP()
	if err != nil {
		panic(err)
	}

	var proxy *Proxy
	if config.HTTPProxyURL != "" || config.HTTPSProxyURL != "" {
		proxy = &Proxy{
			HTTP:    config.HTTPProxyURL,
			HTTPS:   config.HTTPSProxyURL,
			NoProxy: config.NoProxy,
		}
	}

	// Instantiating Datadog primary client
	var ddClients []*Client
	ddClients = append(ddClients, New(
		config.DataDogURL,
		config.DataDogLogIntakeURL,
		config.DataDogAPIKey,
		config.MetricPrefix,
		config.Deployment,
		ipAddress,
		time.Duration(config.DataDogTimeoutSeconds)*time.Second,
		time.Duration(config.FlushDurationSeconds)*time.Second,
		config.FlushMaxBytes,
		log,
		config.CustomTags,
		proxy,
	))
	// Instantiating Additional Datadog endpoints
	i := 0
	for endpoint, keys := range config.DataDogAdditionalEndpoints {
		var logIntakeEndpoint string
		if len(config.DataDogAdditionalLogIntakeEndpoints) > 0 {
			logIntakeEndpoint = config.DataDogAdditionalLogIntakeEndpoints[i]
		}

		for keyIndex := range keys {
			ddClients = append(ddClients, New(
				endpoint,
				logIntakeEndpoint,
				keys[keyIndex],
				config.MetricPrefix,
				config.Deployment,
				ipAddress,
				time.Duration(config.DataDogTimeoutSeconds)*time.Second,
				time.Duration(config.FlushDurationSeconds)*time.Second,
				config.FlushMaxBytes,
				log,
				config.CustomTags,
				proxy,
			))
		}
		i++
	}

	return ddClients, nil
}

// PostLogs forwards the logs to datadog
func (c *Client) PostLogs(logs []logs.LogMessage) uint64 {
	c.log.Debugf("Posting %d logs to account %s", len(logs), c.apiKey[len(c.apiKey)-4:])

	logsData := c.formatter.FormatLogs(c.prefix, c.maxPostBytes, logs)

	unsentLogs := uint64(0)
	for _, entry := range logsData {
		if err := c.postLogs(entry.data); err != nil {
			unsentLogs += entry.nbrItems
			c.log.Errorf("Error posting logs: %s\n\n", err)
		}
	}

	return unsentLogs
}

func (c *Client) postLogs(logsBytes []byte) error {
	url, err := c.logsURL()
	if err != nil {
		return err
	}

	req, err := retryablehttp.NewRequest("POST", url, logsBytes)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "deflate") // Additional header for zlib compression
	req.Header.Set("DD-API-KEY", c.apiKey)

	// If an error is returned by the client (connection errors, etc.), or if a 500-range
	// response code is received, then a retry is invoked on this request after a wait period
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Errorf("error returned by the http client, %v", err)
		return err
	}
	defer resp.Body.Close()

	// Handle errors that occurred even after the retries
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("failed to read body")
		}
		return fmt.Errorf("datadog request returned HTTP response: %s\nResponse Body: %s", resp.Status, body)
	}

	return nil
}

func (c *Client) logsURL() (string, error) {
	logIntakeURL, err := url.Parse(c.logIntakeURL)
	if err != nil {
		return "", fmt.Errorf("error parsing LogIntake API URL %s: %v", c.logIntakeURL, err)
	}
	if !strings.Contains(logIntakeURL.EscapedPath(), "api/v2/logs") {
		logIntakeURL.Path = path.Join(logIntakeURL.Path, "api/v2/logs")
	}
	return logIntakeURL.String(), nil
}

// PostMetrics forwards the metrics to datadog
func (c *Client) PostMetrics(metrics metric.MetricsMap) uint64 {
	c.log.Debugf("Posting %d metrics to account %s", len(metrics), c.apiKey[len(c.apiKey)-4:])
	seriesData := c.formatter.Format(c.prefix, c.maxPostBytes, metrics)
	unsentMetrics := uint64(0)
	for _, entry := range seriesData {
		if entry.data == nil {
			unsentMetrics++
			continue
		}

		if err := c.postMetrics(entry.data); err != nil {
			unsentMetrics += entry.nbrItems
			c.log.Errorf("Error posting metrics: %s\n\n", err)
		}
	}

	return unsentMetrics
}

func (c *Client) postMetrics(seriesBytes []byte) error {
	url, err := c.seriesURL()
	if err != nil {
		return err
	}

	req, err := retryablehttp.NewRequest("POST", url, seriesBytes)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "deflate") // Additional header for zlib compression

	// If an error is returned by the client (connection errors, etc.), or if a 500-range
	// response code is received, then a retry is invoked on this request after a wait period
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Errorf("error returned by the http client, %v", err)
		return err
	}
	defer resp.Body.Close()

	// Handle errors that occurred even after the retries
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			body = []byte("failed to read body")
		}
		return fmt.Errorf("datadog request returned HTTP response: %s\nResponse Body: %s", resp.Status, body)
	}

	return nil
}

func (c *Client) seriesURL() (string, error) {
	apiURL, err := url.Parse(c.apiURL)
	if err != nil {
		return "", fmt.Errorf("error parsing API URL %s: %v", c.apiURL, err)
	}
	if !strings.Contains(apiURL.EscapedPath(), "api/v1/series") {
		apiURL.Path = path.Join(apiURL.Path, "api/v1/series")
	}
	query := apiURL.Query()
	// TODO: send api key in the headers
	query.Add("api_key", c.apiKey)
	apiURL.RawQuery = query.Encode()
	return apiURL.String(), nil
}

// MakeInternalMetric creates a metric with the provided name, value and timestamp
func (c *Client) MakeInternalMetric(name, _type string, value uint64, timestamp int64) (metric.MetricKey, metric.MetricValue) {
	point := metric.Point{
		Timestamp: timestamp,
		Value:     float64(value),
	}

	tags := []string{
		fmt.Sprintf("deployment:%s", c.deployment),
		fmt.Sprintf("ip:%s", c.ip),
	}
	tags = append(tags, c.customTags...)

	key := metric.MetricKey{
		Name:     name,
		TagsHash: util.HashTags(tags),
	}

	mValue := metric.MetricValue{
		Tags:   tags,
		Points: []metric.Point{point},
		Type:   _type,
	}

	return key, mValue
}

// GetProxyTransportFunc manages the proxy configuration
func GetProxyTransportFunc(proxy *Proxy, logger *gosteno.Logger) func(*http.Request) (*url.URL, error) {
	return func(r *http.Request) (*url.URL, error) {
		var proxyURL string
		var requestHost = r.URL.Hostname()
		if r.URL.Scheme == "http" {
			proxyURL = proxy.HTTP
		} else if r.URL.Scheme == "https" {
			proxyURL = proxy.HTTPS
		} else {
			logger.Warnf("Proxy configuration does not support scheme '%s'", r.URL.Scheme)
			return nil, nil // no proxy set
		}

		// This is lightly adapted from https://golang.org/src/net/http/transport.go
		for _, p := range proxy.NoProxy {
			p = strings.ToLower(strings.TrimSpace(p))
			if len(p) == 0 {
				continue
			}
			if hasPort(p) {
				p = p[:strings.LastIndex(p, ":")]
			}
			if requestHost == p {
				return nil, nil
			}
			if len(p) == 0 {
				// There is no host part, likely the entry is malformed; ignore.
				continue
			}
			if p[0] == '.' && (strings.HasSuffix(requestHost, p) || requestHost == p[1:]) {
				// no_proxy ".foo.com" matches "bar.foo.com" or "foo.com"
				return nil, nil
			}
			if p[0] != '.' && strings.HasSuffix(requestHost, p) && requestHost[len(requestHost)-len(p)-1] == '.' {
				// no_proxy "foo.com" matches "bar.foo.com"
				return nil, nil
			}
		}

		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			logger.Errorf("Could not parse the configured %s proxy URL: %s", r.URL.Scheme, err)
			return nil, fmt.Errorf("could not parse the configured %s proxy URL: %s", r.URL.Scheme, err)
		}

		// Clean up the proxy URL for logging
		userInfo := ""
		if parsedURL.User != nil {
			if _, isSet := parsedURL.User.Password(); isSet {
				userInfo = "*****:*****@"
			} else {
				userInfo = "*****@"
			}
		}

		logger.Debugf("Using proxy %s://%s%s for URL '%s'", parsedURL.Scheme, userInfo, parsedURL.Host, r.URL)
		return parsedURL, nil
	}
}

// This utility function is taken from https://golang.org/src/net/http/http.go
func hasPort(s string) bool {
	return strings.LastIndex(s, ":") > strings.LastIndex(s, "]")
}

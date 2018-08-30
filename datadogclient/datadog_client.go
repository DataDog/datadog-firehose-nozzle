package datadogclient

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"io/ioutil"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/gosteno"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type Client struct {
	apiURL              string
	apiKey              string
	prefix              string
	deployment          string
	ip                  string
	customTags          []string
	httpClient          *retryablehttp.Client
	maxPostBytes        uint32
	log                 *gosteno.Logger
	formatter           Formatter
}

type Payload struct {
	Series []metrics.Series `json:"series"`
}

type Proxy struct {
	HTTP    string
	HTTPS   string
	NoProxy []string
}

func New(
	apiURL string,
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
	httpClient.RequestLogHook = func(l *log.Logger, req *http.Request, attemptNum int) {
		if attemptNum == 0 {
			return
		}
		retriesLeft := httpClient.RetryMax - attemptNum
		timeToWait := httpClient.Backoff(httpClient.RetryWaitMin, httpClient.RetryWaitMax, attemptNum-1, nil)
		msg := fmt.Sprintf("Error: %s %s request failed. Wait before retrying: %s (%v left)", req.Method, req.URL, timeToWait, retriesLeft)
		logger.Debug(msg)
	}

	return &Client{
		apiURL: apiURL,
		apiKey: apiKey,
		prefix: prefix,
		deployment: deployment,
		ip: ip,
		log: logger,
		customTags: customTags,
		httpClient: httpClient,
		maxPostBytes: maxPostBytes,
		formatter: Formatter{
			log: logger,
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
			return err
		}
	}

	return nil
}

func (c *Client) postMetrics(seriesBytes []byte) error {
	url := c.seriesURL()

	req, err := retryablehttp.NewRequest("POST", url, bytes.NewReader(seriesBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// If an error is returned by the client (connection errors, etc.), or if a 500-range
	// response code is received, then a retry is invoked on this request after a wait period
	resp, err := c.httpClient.Do(req)
	if err != nil {
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

func (c *Client) seriesURL() string {
	url := fmt.Sprintf("%s?api_key=%s", c.apiURL, c.apiKey)
	return url
}

func (c *Client) MakeInternalMetric(name string, value uint64, timestamp int64) (metrics.MetricKey, metrics.MetricValue) {
	point := metrics.Point{
		Timestamp: timestamp,
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

	return key, mValue
}

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
			return nil, fmt.Errorf("Could not parse the configured %s proxy URL: %s", r.URL.Scheme, err)
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

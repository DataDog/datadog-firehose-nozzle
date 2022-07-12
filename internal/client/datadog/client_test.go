package datadog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/cloudfoundry/gosteno"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/logs"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/DataDog/datadog-firehose-nozzle/test/helper"
)

var (
	bodies       [][]byte
	reqs         chan *http.Request
	responseCode int
	responseBody []byte
	ts           *httptest.Server
	c            *Client
	metricsMap   metric.MetricsMap
	defaultTags  = []string{
		"deployment: test-deployment",
		"job: doppler",
		"origin: test-origin",
		"name: test-origin",
		"ip: dummy-ip",
	}
)

var _ = Describe("DatadogClient", func() {
	BeforeEach(func() {
		bodies = nil
		reqs = make(chan *http.Request, 1000)
		responseCode = http.StatusOK
		responseBody = []byte("some-response-body")
		ts = httptest.NewServer(http.HandlerFunc(handlePost))
		metricsMap = make(metric.MetricsMap)

		c = New(
			ts.URL,
			ts.URL,
			"dummykey",
			"datadog.nozzle.",
			"test-deployment",
			"dummy-ip",
			time.Second,
			2*time.Second,
			2000,
			gosteno.NewLogger("datadogclient test"),
			[]string{},
			nil,
		)
	})

	Context("It parses configured URL correctly", func() {
		It("appends api/v1/series if not present", func() {
			// With trailing slash
			c.apiURL = "https://app.datadoghq.com/"
			result, err := c.seriesURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("https://app.datadoghq.com/api/v1/series"))

			// Without trailing slash
			c.apiURL = "https://app.datadoghq.com"
			result, err = c.seriesURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("https://app.datadoghq.com/api/v1/series"))

		})

		It("doesn't append api/v1/series if present", func() {
			c.apiURL = "https://app.datadoghq.com/api/v1/series"
			result, err := c.seriesURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("https://app.datadoghq.com/api/v1/series"))
		})

		It("keeps query and path intact", func() {
			c.apiURL = "https://app.datadoghq.com/a/path?key=value"
			result, err := c.seriesURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("https://app.datadoghq.com/a/path/api/v1/series?key=value"))
		})

		It("appends api/v2/logs if not present", func() {
			// With trailing slash
			c.logIntakeURL = "http-intake.logs.datadoghq.com/"
			result, err := c.logsURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("http-intake.logs.datadoghq.com/api/v2/logs"))

			// Without trailing slash
			c.logIntakeURL = "http-intake.logs.datadoghq.com"
			result, err = c.logsURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("http-intake.logs.datadoghq.com/api/v2/logs"))
		})

		It("doesn't append api/v2/logs if present", func() {
			c.logIntakeURL = "http-intake.logs.datadoghq.com/api/v2/logs"
			result, err := c.logsURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("http-intake.logs.datadoghq.com/api/v2/logs"))
		})

		It("keeps logs query and path intact", func() {
			c.logIntakeURL = "http-intake.logs.datadoghq.com/a/path?key=value"
			result, err := c.logsURL()
			Expect(err).To(BeNil())
			Expect(result).To(Equal("http-intake.logs.datadoghq.com/a/path/api/v2/logs?key=value"))
		})
	})

	Context("datadog does not respond", func() {
		var fakeBuffer *helper.FakeBufferSink

		BeforeEach(func() {
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var nilChan chan struct{}
				<-nilChan
			}))

			logContent := bytes.NewBuffer(make([]byte, 1024))
			fakeBuffer = helper.NewFakeBufferSink(logContent)
			config := &gosteno.Config{
				Sinks: []gosteno.Sink{
					fakeBuffer,
				},
				Level: gosteno.LOG_DEBUG,
			}
			gosteno.Init(config)

			c = New(
				ts.URL,
				ts.URL,
				"dummykey",
				"datadog.nozzle.",
				"test-deployment",
				"dummy-ip",
				time.Millisecond,
				100*time.Millisecond,
				2000,
				gosteno.NewLogger("datadogclient test"),
				[]string{},
				nil,
			)
		})

		It("respects the timeout for metrics payloads", func() {
			k, v := makeFakeMetric("metricName", "gauge", 1000, 5, defaultTags)
			metricsMap.Add(k, v)

			unsentMetrics := c.PostMetrics(metricsMap)
			Expect(unsentMetrics).ToNot(Equal(uint64(0)))
		})

		It("respects the timeout for logs payloads", func() {
			var data []logs.LogMessage
			lm := makeFakeLogMessage("hostname", "source", "service", "message", "tags")
			data = append(data, lm)

			unsentLogs := c.PostLogs(data)
			Expect(unsentLogs).ToNot(Equal(uint64(0)))
		})

		It("attempts to retry the connection for metrics payloads", func() {
			k, v := makeFakeMetric("metricName", "gauge", 1000, 5, defaultTags)
			metricsMap.Add(k, v)

			unsentMetrics := c.PostMetrics(metricsMap)

			Expect(unsentMetrics).ToNot(Equal(uint64(0)))

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("request failed. Wait before retrying:"))
			Expect(logOutput).To(ContainSubstring("(2 left)"))
			Expect(logOutput).To(ContainSubstring("(1 left)"))
		})

		It("attempts to retry the connection for logs payloads", func() {
			var data []logs.LogMessage
			lm := makeFakeLogMessage("hostname", "source", "service", "message", "tags")
			data = append(data, lm)

			unsentLogs := c.PostLogs(data)
			Expect(unsentLogs).ToNot(Equal(uint64(0)))

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("request failed. Wait before retrying:"))
			Expect(logOutput).To(ContainSubstring("(2 left)"))
			Expect(logOutput).To(ContainSubstring("(1 left)"))
		})
	})

	It("sets Content-Type header when making POST requests", func() {
		k, v := makeFakeMetric("metricName", "gauge", 1000, 5, defaultTags)
		metricsMap.Add(k, v)

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))
		var req *http.Request
		Eventually(reqs).Should(Receive(&req))
		Expect(req.Method).To(Equal("POST"))
		Expect(req.Header.Get("Content-Type")).To(Equal("application/json"))
	})

	It("sends tags", func() {
		k, v := makeFakeMetric("metricName", "gauge", 1000, 5, defaultTags)
		metricsMap.Add(k, v)

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		Eventually(bodies).Should(HaveLen(1))
		var payload Payload
		err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		Expect(payload.Series[0].Tags).To(ConsistOf(
			"deployment: test-deployment",
			"job: doppler",
			"origin: test-origin",
			"name: test-origin",
			"ip: dummy-ip",
		))
	})

	It("creates internal metrics", func() {
		k, v := c.MakeInternalMetric("totalMessagesReceived", metric.GAUGE, 15, time.Now().Unix())
		metricsMap[k] = v

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		Eventually(bodies).Should(HaveLen(1))
		var payload Payload
		err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		Expect(payload.Series[0].Metric).To(Equal("datadog.nozzle.totalMessagesReceived"))
		Expect(payload.Series[0].Tags).To(ConsistOf(
			"ip:dummy-ip",
			"deployment:test-deployment",
		))
		Expect(payload.Series[0].Points).To(HaveLen(1))
		Expect(payload.Series[0].Points[0].Value).To(Equal(float64(15)))
	})

	Context("user configures custom tags", func() {
		BeforeEach(func() {
			c = New(
				ts.URL,
				ts.URL,
				"dummykey",
				"datadog.nozzle.",
				"test-deployment",
				"dummy-ip",
				time.Second,
				2*time.Second,
				2000,
				gosteno.NewLogger("datadogclient test"),
				[]string{"environment:foo", "foundry:bar"},
				nil,
			)
		})

		It("adds custom tags to internal metrics", func() {
			k, v := c.MakeInternalMetric("slowConsumerAlert", metric.GAUGE, 0, time.Now().Unix())
			metricsMap[k] = v

			unsentMetrics := c.PostMetrics(metricsMap)
			Expect(unsentMetrics).To(Equal(uint64(0)))

			Eventually(bodies).Should(HaveLen(1))
			var payload Payload
			err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload.Series).To(HaveLen(1))

			Expect(payload.Series[0].Metric).To(Equal("datadog.nozzle.slowConsumerAlert"))
			Expect(payload.Series[0].Tags).To(ConsistOf(
				"ip:dummy-ip",
				"deployment:test-deployment",
				"environment:foo",
				"foundry:bar",
			))
			Expect(payload.Series[0].Points).To(HaveLen(1))
			Expect(payload.Series[0].Points[0].Value).To(Equal(float64(0)))
		})
	})

	It("uses tags as an identifier for batching purposes (registers metrics with same name and different tags as separate)", func() {
		for i := 0; i < 5; i++ {
			k, v := makeFakeMetric("metricName", "gauge", 1000, uint64(i), []string{"test_tag:1"})
			metricsMap.Add(k, v)
		}
		for i := 0; i < 5; i++ {
			k, v := makeFakeMetric("metricName", "gauge", 1000, uint64(i), []string{"test_tag:2"})
			metricsMap.Add(k, v)
		}

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		Eventually(bodies).Should(HaveLen(1))
		var payload Payload
		err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
		Expect(err).NotTo(HaveOccurred())

		Expect(payload.Series).To(HaveLen(2))

		tag1Found := false
		tag2Found := false
		for _, m := range payload.Series {
			Expect(m.Type).To(Equal("gauge"))

			Expect(m.Tags).To(HaveLen(1))
			if m.Tags[0] == "test_tag:1" {
				tag1Found = true
				Expect(m.Points).To(Equal([]metric.Point{
					{Timestamp: 1000, Value: 0.0},
					{Timestamp: 1000, Value: 1.0},
					{Timestamp: 1000, Value: 2.0},
					{Timestamp: 1000, Value: 3.0},
					{Timestamp: 1000, Value: 4.0},
				}))
			} else if m.Tags[0] == "test_tag:2" {
				tag2Found = true
				Expect(m.Points).To(Equal([]metric.Point{
					{Timestamp: 1000, Value: 0.0},
					{Timestamp: 1000, Value: 1.0},
					{Timestamp: 1000, Value: 2.0},
					{Timestamp: 1000, Value: 3.0},
					{Timestamp: 1000, Value: 4.0},
				}))
			}
		}

		Expect(tag1Found).To(BeTrue())
		Expect(tag2Found).To(BeTrue())
	})

	It("posts ValueMetrics in JSON format & adds the metric prefix", func() {
		k, v := makeFakeMetric("valueName", "gauge", 1, 5, defaultTags)
		metricsMap.Add(k, v)
		k, v = makeFakeMetric("valueName", "gauge", 2, 76, defaultTags)
		metricsMap.Add(k, v)

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		Eventually(bodies).Should(HaveLen(1))

		var payload Payload
		err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		m := payload.Series[0]
		Expect(m.Type).To(Equal("gauge"))
		Expect(m.Metric).To(Equal("datadog.nozzle.valueName"))
		Expect(m.Points).To(Equal([]metric.Point{
			{Timestamp: 1, Value: 5.0},
			{Timestamp: 2, Value: 76.0},
		}))
		Expect(m.Tags).To(Equal(defaultTags))
	})

	It("posts CounterEvents in JSON format & adds the metric prefix", func() {
		k, v := makeFakeMetric("counterName", "gauge", 1, 5, defaultTags)
		metricsMap.Add(k, v)
		k, v = makeFakeMetric("counterName", "gauge", 2, 11, defaultTags)
		metricsMap.Add(k, v)

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		Eventually(bodies).Should(HaveLen(1))

		var payload Payload
		err := json.Unmarshal(helper.Decompress(bodies[0]), &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		m := payload.Series[0]
		Expect(m.Type).To(Equal("gauge"))
		Expect(m.Metric).To(Equal("datadog.nozzle.counterName"))
		Expect(m.Points).To(Equal([]metric.Point{
			{Timestamp: 1, Value: 5.0},
			{Timestamp: 2, Value: 11.0},
		}))
		Expect(m.Tags).To(Equal(defaultTags))
	})

	It("breaks up a metrics message that exceeds the FlushMaxBytes", func() {
		for i := 0; i < 1000; i++ {
			k, v := makeFakeMetric(fmt.Sprintf("metricName_%v", i), "gauge", 1000, 1, defaultTags)
			metricsMap.Add(k, v)
		}
		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))
		f := func() int {
			return len(bodies)
		}
		Eventually(f).Should(BeNumerically(">", 1))
	})

	It("breaks up a logs message that exceeds the FlushMaxBytes", func() {
		var data []logs.LogMessage
		for i := 0; i < 10000; i++ {
			lm := makeFakeLogMessage("hostname", "source", "service", "message", "tags")
			data = append(data, lm)
		}
		unsentLogs := c.PostLogs(data)
		Expect(unsentLogs).To(Equal(uint64(0)))
		f := func() int {
			return len(bodies)
		}
		Eventually(f).Should(BeNumerically(">", 1))
	})

	It("discards metrics that exceed that max size", func() {
		c.maxPostBytes = 10

		name := proto.String(strings.Repeat("some-big-name", 1000))
		k, v := makeFakeMetric(*name, "gauge", 1000, 5, defaultTags)
		metricsMap.Add(k, v)

		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		f := func() int {
			return len(bodies)
		}

		Consistently(f).Should(Equal(0))
	})

	It("discards logs that exceed that max size", func() {
		c.maxPostBytes = 10

		message := strings.Repeat("some-big-message", 1000)
		var data []logs.LogMessage
		lm := makeFakeLogMessage("hostname", "source", "service", message, "tags")
		data = append(data, lm)

		unsentLogs := c.PostLogs(data)
		Expect(unsentLogs).To(Equal(uint64(0)))

		f := func() int {
			return len(bodies)
		}

		Consistently(f).Should(Equal(0))
	})

	It("returns an error when datadog responds with a non 200 response code", func() {
		// Need to add at least 1 value to metrics map for it to send a message
		k, v := c.MakeInternalMetric("test", metric.GAUGE, 5, time.Now().Unix())
		metricsMap[k] = v

		// PostMetrics
		responseCode = http.StatusBadRequest // 400
		responseBody = []byte("something went horribly wrong")
		unsentMetrics := c.PostMetrics(metricsMap)
		Expect(unsentMetrics).ToNot(Equal(uint64(0)))

		responseCode = http.StatusSwitchingProtocols // 101
		unsentMetrics = c.PostMetrics(metricsMap)
		Expect(unsentMetrics).ToNot(Equal(uint64(0)))

		responseCode = http.StatusAccepted // 201
		unsentMetrics = c.PostMetrics(metricsMap)
		Expect(unsentMetrics).To(Equal(uint64(0)))

		var data []logs.LogMessage
		lm := makeFakeLogMessage("hostname", "source", "service", "message", "tags")
		data = append(data, lm)

		// PostLogs
		responseCode = http.StatusBadRequest // 400
		responseBody = []byte("something went horribly wrong")
		unsentLogs := c.PostLogs(data)
		Expect(unsentLogs).ToNot(Equal(uint64(0)))

		responseCode = http.StatusSwitchingProtocols // 101
		unsentLogs = c.PostLogs(data)
		Expect(unsentLogs).ToNot(Equal(uint64(0)))

		responseCode = http.StatusAccepted // 201
		unsentLogs = c.PostLogs(data)
		Expect(unsentLogs).To(Equal(uint64(0)))
	})

	It("parses proxy URLs correctly & chooses the correct proxy to use by scheme", func() {
		println("proxy test")
		proxy := &Proxy{
			HTTP:    "http://user:password@host.com:1234",
			HTTPS:   "https://user:password@host.com:1234",
			NoProxy: []string{"datadoghq.com"},
		}

		rHTTP, _ := http.NewRequest("GET", "http://test.com", nil)
		rHTTPS, _ := http.NewRequest("GET", "https://test.com", nil)
		rHTTPNoProxy, _ := http.NewRequest("GET", "http://datadoghq.com", nil)
		rHTTPSNoProxy, _ := http.NewRequest("GET", "https://datadoghq.com", nil)

		proxyFunc := GetProxyTransportFunc(proxy, gosteno.NewLogger("test"))

		proxyURL, err := proxyFunc(rHTTP)
		Expect(err).To(BeNil())
		Expect(proxyURL.String()).To(Equal("http://user:password@host.com:1234"))
		proxyURL, err = proxyFunc(rHTTPS)
		Expect(err).To(BeNil())
		Expect(proxyURL.String()).To(Equal("https://user:password@host.com:1234"))

		proxyURL, err = proxyFunc(rHTTPNoProxy)
		Expect(err).To(BeNil())
		Expect(proxyURL).To(BeNil())
		proxyURL, err = proxyFunc(rHTTPSNoProxy)
		Expect(err).To(BeNil())
		Expect(proxyURL).To(BeNil())
	})

	It("errors when a bad proxy URL is set", func() {
		proxy := &Proxy{
			HTTP:  "1234://bad_url",
			HTTPS: "1234s://still_a_bad_url",
		}

		rHTTP, _ := http.NewRequest("GET", "http://test.com", nil)
		rHTTPS, _ := http.NewRequest("GET", "https://test.com", nil)

		proxyFunc := GetProxyTransportFunc(proxy, gosteno.NewLogger("datadogclient test"))

		proxyURL, err := proxyFunc(rHTTP)
		Expect(err).ToNot(BeNil())
		Expect(proxyURL).To(BeNil())

		proxyURL, err = proxyFunc(rHTTPS)
		Expect(err).ToNot(BeNil())
		Expect(proxyURL).To(BeNil())
	})

	It("doesn't set a proxy when an unsupported scheme is used", func() {
		proxy := &Proxy{
			HTTP:  "http://user@password@host.com@port",
			HTTPS: "https://user@password@host.com@port",
		}

		rWS, _ := http.NewRequest("GET", "ws://test.com", nil)

		proxyFunc := GetProxyTransportFunc(proxy, gosteno.NewLogger("datadogclient test"))

		proxyURL, err := proxyFunc(rWS)
		Expect(err).To(BeNil())
		Expect(proxyURL).To(BeNil())
	})
})

func handlePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic("No body!")
	}

	reqs <- r
	bodies = append(bodies, body)
	w.WriteHeader(responseCode)
	w.Write(responseBody)
}

func makeFakeMetric(name, _type string, timeStamp, value uint64, tags []string) (metric.MetricKey, metric.MetricValue) {
	key := metric.MetricKey{
		Name:     name,
		TagsHash: util.HashTags(tags),
	}

	point := metric.Point{
		Timestamp: int64(timeStamp),
		Value:     float64(value),
	}

	mValue := metric.MetricValue{
		Host:   "test-origin",
		Tags:   tags,
		Points: []metric.Point{point},
		Type:   _type,
	}

	return key, mValue
}

func makeFakeLogMessage(hostname, source, service, message, tags string) logs.LogMessage {
	return logs.LogMessage{
		Hostname: hostname,
		Source:   source,
		Service:  service,
		Tags:     tags,
		Message:  message,
	}
}

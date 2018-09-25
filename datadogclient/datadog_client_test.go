package datadogclient_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/testhelpers"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
)

var (
	bodies       [][]byte
	reqs         chan *http.Request
	responseCode int
	responseBody []byte
	ts           *httptest.Server
	c            *datadogclient.Client
	metricsMap   metrics.MetricsMap
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
		metricsMap = make(metrics.MetricsMap)

		c = datadogclient.New(
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

	Context("datadog does not respond", func() {
		var fakeBuffer *testhelpers.FakeBufferSink

		BeforeEach(func() {
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var nilChan chan struct{}
				<-nilChan
			}))

			logContent := bytes.NewBuffer(make([]byte, 1024))
			fakeBuffer = testhelpers.NewFakeBufferSink(logContent)
			config := &gosteno.Config{
				Sinks: []gosteno.Sink{
					fakeBuffer,
				},
				Level: gosteno.LOG_DEBUG,
			}
			gosteno.Init(config)

			c = datadogclient.New(
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

		It("respects the timeout", func() {
			k, v := makeFakeMetric("metricName", 1000, 5, events.Envelope_ValueMetric, defaultTags)
			metricsMap.Add(k, v)

			errs := make(chan error)
			go func() {
				errs <- c.PostMetrics(metricsMap)
			}()
			Eventually(errs).Should(Receive(HaveOccurred()))
		})

		It("attempts to retry the connection", func() {
			k, v := makeFakeMetric("metricName", 1000, 5, events.Envelope_ValueMetric, defaultTags)
			metricsMap.Add(k, v)

			errs := make(chan error)
			go func() {
				errs <- c.PostMetrics(metricsMap)
			}()

			var err error
			Eventually(errs).Should(Receive(&err))
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("giving up after 4 attempts"))

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("request failed. Wait before retrying:"))
			Expect(logOutput).To(ContainSubstring("(2 left)"))
			Expect(logOutput).To(ContainSubstring("(1 left)"))
		})
	})

	It("sets Content-Type header when making POST requests", func() {
		k, v := makeFakeMetric("metricName", 1000, 5, events.Envelope_ValueMetric, defaultTags)
		metricsMap.Add(k, v)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())
		var req *http.Request
		Eventually(reqs).Should(Receive(&req))
		Expect(req.Method).To(Equal("POST"))
		Expect(req.Header.Get("Content-Type")).To(Equal("application/json"))
	})

	It("sends tags", func() {
		k, v := makeFakeMetric("metricName", 1000, 5, events.Envelope_ValueMetric, defaultTags)
		metricsMap.Add(k, v)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))
		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
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
		k, v := c.MakeInternalMetric("totalMessagesReceived", 15, time.Now().Unix())
		metricsMap[k] = v

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))
		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
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
			c = datadogclient.New(
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
			k, v := c.MakeInternalMetric("slowConsumerAlert", 0, time.Now().Unix())
			metricsMap[k] = v

			err := c.PostMetrics(metricsMap)
			Expect(err).ToNot(HaveOccurred())

			Eventually(bodies).Should(HaveLen(1))
			var payload datadogclient.Payload
			err = json.Unmarshal(bodies[0], &payload)
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
			k, v := makeFakeMetric("metricName", 1000, uint64(i), events.Envelope_ValueMetric, []string{"test_tag:1"})
			metricsMap.Add(k, v)
		}
		for i := 0; i < 5; i++ {
			k, v := makeFakeMetric("metricName", 1000, uint64(i), events.Envelope_ValueMetric, []string{"test_tag:2"})
			metricsMap.Add(k, v)
		}

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))
		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())

		Expect(payload.Series).To(HaveLen(2))

		tag1Found := false
		tag2Found := false
		for _, metric := range payload.Series {
			Expect(metric.Type).To(Equal("gauge"))

			Expect(metric.Tags).To(HaveLen(1))
			if metric.Tags[0] == "test_tag:1" {
				tag1Found = true
				Expect(metric.Points).To(Equal([]metrics.Point{
					{Timestamp: 1000, Value: 0.0},
					{Timestamp: 1000, Value: 1.0},
					{Timestamp: 1000, Value: 2.0},
					{Timestamp: 1000, Value: 3.0},
					{Timestamp: 1000, Value: 4.0},
				}))
			} else if metric.Tags[0] == "test_tag:2" {
				tag2Found = true
				Expect(metric.Points).To(Equal([]metrics.Point{
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
		k, v := makeFakeMetric("valueName", 1, 5, events.Envelope_ValueMetric, defaultTags)
		metricsMap.Add(k, v)
		k, v = makeFakeMetric("valueName", 2, 76, events.Envelope_ValueMetric, defaultTags)
		metricsMap.Add(k, v)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))

		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		metric := payload.Series[0]
		Expect(metric.Type).To(Equal("gauge"))
		Expect(metric.Metric).To(Equal("datadog.nozzle.valueName"))
		Expect(metric.Points).To(Equal([]metrics.Point{
			{Timestamp: 1, Value: 5.0},
			{Timestamp: 2, Value: 76.0},
		}))
		Expect(metric.Tags).To(Equal(defaultTags))
	})

	It("posts CounterEvents in JSON format & adds the metric prefix", func() {
		k, v := makeFakeMetric("counterName", 1, 5, events.Envelope_CounterEvent, defaultTags)
		metricsMap.Add(k, v)
		k, v = makeFakeMetric("counterName", 2, 11, events.Envelope_CounterEvent, defaultTags)
		metricsMap.Add(k, v)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))

		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(1))

		metric := payload.Series[0]
		Expect(metric.Type).To(Equal("gauge"))
		Expect(metric.Metric).To(Equal("datadog.nozzle.counterName"))
		Expect(metric.Points).To(Equal([]metrics.Point{
			{Timestamp: 1, Value: 5.0},
			{Timestamp: 2, Value: 11.0},
		}))
		Expect(metric.Tags).To(Equal(defaultTags))
	})

	It("breaks up a message that exceeds the FlushMaxBytes", func() {
		for i := 0; i < 1000; i++ {
			k, v := makeFakeMetric("metricName", 1000, uint64(i), events.Envelope_ValueMetric, defaultTags)
			metricsMap.Add(k, v)
		}

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		f := func() int {
			return len(bodies)
		}

		Eventually(f).Should(BeNumerically(">", 1))
	})

	It("discards metrics that exceed that max size", func() {
		name := proto.String(strings.Repeat("some-big-name", 1000))
		k, v := makeFakeMetric(*name, 1000, 5, events.Envelope_ValueMetric, defaultTags)
		metricsMap.Add(k, v)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		f := func() int {
			return len(bodies)
		}

		Consistently(f).Should(Equal(0))
	})

	It("returns an error when datadog responds with a non 200 response code", func() {
		// Need to add at least 1 value to metrics map for it to send a message
		k, v := c.MakeInternalMetric("test", 5, time.Now().Unix())
		metricsMap[k] = v

		responseCode = http.StatusBadRequest // 400
		responseBody = []byte("something went horribly wrong")
		err := c.PostMetrics(metricsMap)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("datadog request returned HTTP response: 400 Bad Request"))
		Expect(err.Error()).To(ContainSubstring("something went horribly wrong"))

		responseCode = http.StatusSwitchingProtocols // 101
		err = c.PostMetrics(metricsMap)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("datadog request returned HTTP response: 101"))

		responseCode = http.StatusAccepted // 201
		err = c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())
	})

	It("parses proxy URLs correctly & chooses the correct proxy to use by scheme", func() {
		println("proxy test")
		proxy := &datadogclient.Proxy{
			HTTP:    "http://user:password@host.com:port",
			HTTPS:   "https://user:password@host.com:port",
			NoProxy: []string{"datadoghq.com"},
		}

		rHTTP, _ := http.NewRequest("GET", "http://test.com", nil)
		rHTTPS, _ := http.NewRequest("GET", "https://test.com", nil)
		rHTTPNoProxy, _ := http.NewRequest("GET", "http://datadoghq.com", nil)
		rHTTPSNoProxy, _ := http.NewRequest("GET", "https://datadoghq.com", nil)

		proxyFunc := datadogclient.GetProxyTransportFunc(proxy, gosteno.NewLogger("datadogclient test"))

		proxyURL, err := proxyFunc(rHTTP)
		Expect(err).To(BeNil())
		Expect(proxyURL.String()).To(Equal("http://user:password@host.com:port"))
		proxyURL, err = proxyFunc(rHTTPS)
		Expect(err).To(BeNil())
		Expect(proxyURL.String()).To(Equal("https://user:password@host.com:port"))

		proxyURL, err = proxyFunc(rHTTPNoProxy)
		Expect(err).To(BeNil())
		Expect(proxyURL).To(BeNil())
		proxyURL, err = proxyFunc(rHTTPSNoProxy)
		Expect(err).To(BeNil())
		Expect(proxyURL).To(BeNil())
	})

	It("errors when a bad proxy URL is set", func() {
		proxy := &datadogclient.Proxy{
			HTTP:  "1234://bad_url",
			HTTPS: "1234s://still_a_bad_url",
		}

		rHTTP, _ := http.NewRequest("GET", "http://test.com", nil)
		rHTTPS, _ := http.NewRequest("GET", "https://test.com", nil)

		proxyFunc := datadogclient.GetProxyTransportFunc(proxy, gosteno.NewLogger("datadogclient test"))

		proxyURL, err := proxyFunc(rHTTP)
		Expect(err).ToNot(BeNil())
		Expect(proxyURL).To(BeNil())

		proxyURL, err = proxyFunc(rHTTPS)
		Expect(err).ToNot(BeNil())
		Expect(proxyURL).To(BeNil())
	})

	It("doesn't set a proxy when an unsupported scheme is used", func() {
		proxy := &datadogclient.Proxy{
			HTTP:  "http://user@password@host.com@port",
			HTTPS: "https://user@password@host.com@port",
		}

		rWS, _ := http.NewRequest("GET", "ws://test.com", nil)

		proxyFunc := datadogclient.GetProxyTransportFunc(proxy, gosteno.NewLogger("datadogclient test"))

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

func makeFakeMetric(name string, timeStamp, value uint64, eventType events.Envelope_EventType, tags []string) (metrics.MetricKey, metrics.MetricValue) {
	key := metrics.MetricKey{
		Name:      name,
		TagsHash:  utils.HashTags(tags),
		EventType: eventType,
	}

	point := metrics.Point{
		Timestamp: int64(timeStamp),
		Value:     float64(value),
	}

	mValue := metrics.MetricValue{
		Host:   "test-origin",
		Tags:   tags,
		Points: []metrics.Point{point},
	}

	return key, mValue
}

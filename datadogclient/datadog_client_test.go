package datadogclient_test

import (
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
	"github.com/DataDog/datadog-firehose-nozzle/metricProcessor"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
)

var (
	bodies       [][]byte
	reqs         chan *http.Request
	responseCode int
	responseBody []byte
	ts           *httptest.Server
	c            *datadogclient.Client
	p            *metricProcessor.Processor
	mchan        chan []metrics.MetricPackage
	metricsMap   metrics.MetricsMap
)

var _ = Describe("DatadogClient", func() {
	BeforeEach(func() {
		bodies = nil
		reqs = make(chan *http.Request, 1000)
		responseCode = http.StatusOK
		responseBody = []byte("some-response-body")
		ts = httptest.NewServer(http.HandlerFunc(handlePost))
		mchan = make(chan []metrics.MetricPackage, 1500)
		p = metricProcessor.New(mchan)
		metricsMap = make(metrics.MetricsMap)

		c = datadogclient.New(
			ts.URL,
			"dummykey",
			"datadog.nozzle.",
			"test-deployment",
			"dummy-ip",
			time.Second,
			1500,
			gosteno.NewLogger("datadogclient test"),
		)
	})

	Context("datadog does not respond", func() {
		BeforeEach(func() {
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var nilChan chan struct{}
				<-nilChan
			}))
			c = datadogclient.New(
				ts.URL,
				"dummykey",
				"datadog.nozzle.",
				"test-deployment",
				"dummy-ip",
				time.Millisecond,
				1024,
				gosteno.NewLogger("datadogclient test"),
			)
		})

		It("respects the timeout", func() {
			p.ProcessMetric(&events.Envelope{
				Origin:    proto.String("test-origin"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_ValueMetric.Enum(),

				// fields that gets sent as tags
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
				Index:      proto.String("1"),
				Ip:         proto.String("10.0.1.2"),
			})

			receiveMessages(1)

			errs := make(chan error)
			go func() {
				errs <- c.PostMetrics(metricsMap)
			}()
			Eventually(errs).Should(Receive(HaveOccurred()))
		})
	})

	It("sets Content-Type header when making POST requests", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
			Index:      proto.String("1"),
			Ip:         proto.String("10.0.1.2"),
		})

		receiveMessages(1)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())
		var req *http.Request
		Eventually(reqs).Should(Receive(&req))
		Expect(req.Method).To(Equal("POST"))
		Expect(req.Header.Get("Content-Type")).To(Equal("application/json"))
	})

	It("sends tags", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
			Index:      proto.String("1"),
			Ip:         proto.String("10.0.1.2"),

			// additional tags
			Tags: map[string]string{
				"protocol":   "http",
				"request_id": "a1f5-deadbeef",
			},
		})

		receiveMessages(1)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))
		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(2))

		var metric metrics.Series
		Expect(payload.Series).To(ContainMetric("datadog.nozzle.test-origin.", &metric))
		Expect(metric.Tags).To(ConsistOf(
			"deployment:deployment-name",
			"job:doppler",
			"index:1",
			"ip:10.0.1.2",
			"protocol:http",
			"name:test-origin",
			"origin:test-origin",
			"request_id:a1f5-deadbeef",
		))
		Expect(payload.Series).To(ContainMetric("datadog.nozzle.", &metric))
		Expect(metric.Tags).To(ConsistOf(
			"deployment:deployment-name",
			"job:doppler",
			"index:1",
			"ip:10.0.1.2",
			"protocol:http",
			"name:test-origin",
			"origin:test-origin",
			"request_id:a1f5-deadbeef",
		))
	})

	It("uses tags as an identifier for batching purposes", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
			Index:      proto.String("1"),
			Ip:         proto.String("10.0.1.2"),

			// additional tags
			Tags: map[string]string{
				"protocol":   "http",
				"request_id": "a1f5-deadbeef",
			},
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
			Index:      proto.String("1"),
			Ip:         proto.String("10.0.1.2"),

			// additional tags
			Tags: map[string]string{
				"protocol":   "https",
				"request_id": "d3ac-livefood",
			},
		})

		receiveMessages(2)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())
		Eventually(bodies).Should(HaveLen(1))
		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(4))
		Expect(payload.Series).To(ContainMetricWithTags(
			"datadog.nozzle.test-origin.",
			"deployment:deployment-name",
			"index:1",
			"ip:10.0.1.2",
			"job:doppler",
			"name:test-origin",
			"origin:test-origin",
			"protocol:https",
			"request_id:d3ac-livefood",
		))
		Expect(payload.Series).To(ContainMetricWithTags(
			"datadog.nozzle.test-origin.",
			"deployment:deployment-name",
			"index:1",
			"ip:10.0.1.2",
			"job:doppler",
			"name:test-origin",
			"origin:test-origin",
			"protocol:http",
			"request_id:a1f5-deadbeef",
		))
		Expect(payload.Series).To(ContainMetricWithTags(
			"datadog.nozzle.",
			"deployment:deployment-name",
			"index:1",
			"ip:10.0.1.2",
			"job:doppler",
			"name:test-origin",
			"origin:test-origin",
			"protocol:https",
			"request_id:d3ac-livefood",
		))
		Expect(payload.Series).To(ContainMetricWithTags(
			"datadog.nozzle.",
			"deployment:deployment-name",
			"index:1",
			"ip:10.0.1.2",
			"job:doppler",
			"name:test-origin",
			"origin:test-origin",
			"protocol:http",
			"request_id:a1f5-deadbeef",
		))
	})

	It("posts ValueMetrics in JSON format", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(2000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(76),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		receiveMessages(2)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))

		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(2))

		metricsFound := 0
		for _, metric := range payload.Series {
			Expect(metric.Type).To(Equal("gauge"))

			if metric.Metric == "datadog.nozzle.origin.metricName" || metric.Metric == "datadog.nozzle.metricName" {
				metricsFound++
				Expect(metric.Points).To(Equal([]metrics.Point{
					metrics.Point{
						Timestamp: 1,
						Value:     5.0,
					},
					metrics.Point{
						Timestamp: 2,
						Value:     76.0,
					},
				}))
				Expect(metric.Tags).To(Equal([]string{
					"deployment:deployment-name",
					"job:doppler",
					"name:origin",
					"origin:origin",
				}))
			}
		}
		Expect(metricsFound).To(Equal(2))
	})

	It("breaks up a message that exceeds the FlushMaxBytes", func() {
		for i := 0; i < 1000; i++ {
			p.ProcessMetric(&events.Envelope{
				Origin:    proto.String("origin"),
				Timestamp: proto.Int64(1000000000 + int64(i)),
				EventType: events.Envelope_ValueMetric.Enum(),
				ValueMetric: &events.ValueMetric{
					Name:  proto.String("metricName"),
					Value: proto.Float64(5),
				},
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
			})
		}

		receiveMessages(1000)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		f := func() int {
			return len(bodies)
		}

		Eventually(f).Should(BeNumerically(">", 1))
	})

	It("discards metrics that exceed that max size", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String(strings.Repeat("some-big-name", 1000)),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		receiveMessages(1)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		f := func() int {
			return len(bodies)
		}

		Consistently(f).Should(Equal(0))
	})

	It("registers metrics with the same name but different tags as different", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(2000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(76),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("gorouter"),
		})

		receiveMessages(2)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))

		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(4))
		dopplerFound := false
		gorouterFound := false
		for _, metric := range payload.Series {
			Expect(metric.Type).To(Equal("gauge"))

			if metric.Metric == "datadog.nozzle.origin.metricName" {
				Expect(metric.Tags).To(HaveLen(4))
				Expect(metric.Tags[0]).To(Equal("deployment:deployment-name"))
				if metric.Tags[1] == "job:doppler" {
					dopplerFound = true
					Expect(metric.Points).To(Equal([]metrics.Point{
						metrics.Point{
							Timestamp: 1,
							Value:     5.0,
						},
					}))
				} else if metric.Tags[1] == "job:gorouter" {
					gorouterFound = true
					Expect(metric.Points).To(Equal([]metrics.Point{
						metrics.Point{
							Timestamp: 2,
							Value:     76.0,
						},
					}))
				} else {
					panic("Unknown tag found")
				}
			}
		}
		Expect(dopplerFound).To(BeTrue())
		Expect(gorouterFound).To(BeTrue())
	})

	It("posts CounterEvents in JSON format", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_CounterEvent.Enum(),
			CounterEvent: &events.CounterEvent{
				Name:  proto.String("counterName"),
				Delta: proto.Uint64(1),
				Total: proto.Uint64(5),
			},
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(2000000000),
			EventType: events.Envelope_CounterEvent.Enum(),
			CounterEvent: &events.CounterEvent{
				Name:  proto.String("counterName"),
				Delta: proto.Uint64(6),
				Total: proto.Uint64(11),
			},
		})

		receiveMessages(2)

		err := c.PostMetrics(metricsMap)
		Expect(err).ToNot(HaveOccurred())

		Eventually(bodies).Should(HaveLen(1))

		var payload datadogclient.Payload
		err = json.Unmarshal(bodies[0], &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Series).To(HaveLen(2))
		counterNameFound := false
		for _, metric := range payload.Series {
			Expect(metric.Type).To(Equal("gauge"))

			if metric.Metric == "datadog.nozzle.origin.counterName" {
				counterNameFound = true
				Expect(metric.Points).To(Equal([]metrics.Point{
					metrics.Point{
						Timestamp: 1,
						Value:     5.0,
					},
					metrics.Point{
						Timestamp: 2,
						Value:     11.0,
					},
				}))
			}
		}
		Expect(counterNameFound).To(BeTrue())
	})

	It("returns an error when datadog responds with a non 200 response code", func() {
		// Need to add at least 1 value to metrics map for it to send a message
		k, v := c.MakeInternalMetric("test", 5)
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

func receiveMessages(expecting int) {
	for i := 0; i < expecting; i++ {
		select {
		case metricPkg := <-mchan:
			for _, m := range metricPkg {
				metricsMap.Add(*m.MetricKey, *m.MetricValue)
			}
		}
	}
}

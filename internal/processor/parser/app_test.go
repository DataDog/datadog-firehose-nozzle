package parser

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/gosteno"
)

var _ = Describe("AppMetrics", func() {
	var (
		log                    *gosteno.Logger
		fakeCloudControllerAPI *FakeCloudControllerAPI
		ccAPIURL               string
		fakeCfClient           *cloudfoundry.CFClient
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("datadogclient test")
		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()

		ccAPIURL = fakeCloudControllerAPI.URL()
		cfg := config.Config{
			CloudControllerEndpoint: ccAPIURL,
			Client:                  "bearer",
			ClientSecret:            "123456789",
			InsecureSSLSkipVerify:   true,
			NumWorkers:              5,
		}
		var err error
		fakeCfClient, err = cloudfoundry.NewClient(&cfg, log)
		Expect(err).To(BeNil())
	}, 0)

	Context("generator function", func() {
		It("errors out properly when it cannot connect", func() {
			_, err := NewAppParser(nil, 5, 10, log, []string{}, "")
			Expect(err).NotTo(BeNil())
		})

		It("generates it properly when it can connect", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "")
			Expect(err).To(BeNil())
			Expect(a).NotTo(BeNil())
		})
	})

	Context("cache warmup", func() {
		It("requests all the apps directly at startup", func() {
			a, err := NewAppParser(fakeCfClient, 5, 999, log, []string{}, "")
			Expect(err).To(BeNil())
			Expect(a).NotTo(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			Expect(len(a.AppCache.apps)).To(Equal(14))
		})

		It("does not block while warming cache", func() {
			fakeCloudControllerAPI.RequestTime = 100
			a, err := NewAppParser(fakeCfClient, 5, 999, log, []string{}, "")
			// Assertions are done while cache is warming up in the background
			Expect(err).To(BeNil())
			Expect(a).NotTo(BeNil())
			Expect(a.AppCache.IsWarmedUp()).To(BeFalse())
			// Eventually, the cache is ready
			Eventually(a.AppCache.IsWarmedUp, 10*time.Second).Should(BeTrue())
		})
	})

	Context("app metrics test", func() {
		It("tries to get it from the cloud controller when not in the cache", func() {
			a, _ := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "")
			var req *http.Request
			// Wait for cache warmup to finish
			Eventually(fakeCloudControllerAPI.ReceivedRequests).ShouldNot(Receive())
			_, err := a.getAppData("app-5")
			Expect(err).ToNot(BeNil()) // error expected because fake CC won't return an app, so unmarshalling will fail
			Eventually(fakeCloudControllerAPI.ReceivedRequests).Should(Receive(&req))
			Expect(req.URL.Path).To(Equal("/v2/apps/app-5"))
		})

		It("grabs from the cache when it present", func() {
			a, _ := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "")
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			// 6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a corresponds to hello-datadog-cf-ruby-dev
			Expect(a.AppCache.apps).To(HaveKey("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
			app, err := a.getAppData("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a")
			Expect(err).To(BeNil())
			Expect(app).NotTo(BeNil())
		})
	})

	Context("metric evaluation test", func() {
		It("parses an event properly", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "env_name")
			Expect(err).To(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())

			event := &loggregator_v2.Envelope{
				Timestamp:  1000000000,
				SourceId:   "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
				InstanceId: "4",
				Tags: map[string]string{
					"origin":     "test-origin",
					"deployment": "deployment-name",
					"job":        "doppler",
					"index":      "1",
					"ip":         "10.0.1.2",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": {
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory": {
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk": {
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory_quota": {
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk_quota": {
								Unit:  "gauge",
								Value: float64(1),
							},
						},
					},
				},
			}

			metrics, err := a.Parse(event)

			Expect(err).To(BeNil())
			Expect(metrics).To(HaveLen(10))

			Expect(metrics).To(ContainMetric("app.disk.configured"))
			Expect(metrics).To(ContainMetric("app.disk.provisioned"))
			Expect(metrics).To(ContainMetric("app.memory.configured"))
			Expect(metrics).To(ContainMetric("app.memory.provisioned"))
			Expect(metrics).To(ContainMetric("app.instances"))
			Expect(metrics).To(ContainMetric("app.cpu.pct"))
			Expect(metrics).To(ContainMetric("app.disk.used"))
			Expect(metrics).To(ContainMetric("app.disk.quota"))
			Expect(metrics).To(ContainMetric("app.memory.used"))
			Expect(metrics).To(ContainMetric("app.memory.quota"))

			for _, metric := range metrics {
				Expect(metric.MetricValue.Tags).To(ContainElement("app_name:hello-datadog-cf-ruby-dev"))
				Expect(metric.MetricValue.Tags).To(ContainElement("guid:6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
				Expect(metric.MetricValue.Tags).To(ContainElement("env:env_name"))
				Expect(metric.MetricValue.Tags).To(ContainElement("source_id:6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/al1:alv1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/al2:alv2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/gl1:app"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/aa1:aav1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/aa2:aav2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/ga1:app"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/sl1:slv1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/sl2:slv2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/gl2:space"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/sa1:sav1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/sa2:sav2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/ga2:space"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/ol1:olv1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/ol2:olv2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/gl3:org"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/oa1:oav1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/oa2:oav2"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/ga3:org"))
			}
		})
	})

	Context("expected tags", func() {
		It("adds proper instance tag", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "env_name")
			Expect(err).To(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())

			event := &loggregator_v2.Envelope{
				Timestamp:  1000000000,
				SourceId:   "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
				InstanceId: "4",
				Tags: map[string]string{
					"origin":     "test-origin",
					"deployment": "deployment-name",
					"job":        "doppler",
					"index":      "1",
					"ip":         "10.0.1.2",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
						},
					},
				},
			}

			metricsWithInstanceTag := map[string]bool{
				"app.cpu.pct":      true,
				"app.disk.used":    true,
				"app.disk.quota":   true,
				"app.memory.used":  true,
				"app.memory.quota": true,
			}

			metrics, err := a.Parse(event)
			Expect(metrics).To(HaveLen(10))
			for _, metric := range metrics {
				if metricsWithInstanceTag[metric.MetricKey.Name] {
					Expect(metric.MetricValue.Tags).To(ContainElement("instance:4"))
				}
			}

			// instance_index should be preferred over InstanceId
			event.GetGauge().GetMetrics()["instance_index"] = &loggregator_v2.GaugeValue{Value: float64(3)}
			metrics, err = a.Parse(event)
			Expect(metrics).To(HaveLen(10))
			for _, metric := range metrics {
				if metricsWithInstanceTag[metric.MetricKey.Name] {
					Expect(metric.MetricValue.Tags).To(ContainElement("instance:3"))
				}
			}
		})
	})

	Context("custom tags", func() {
		It("attaches custom tags if present", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{"custom:tag", "foo:bar"},
				"env_name")
			Expect(err).To(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())

			event := &loggregator_v2.Envelope{
				Timestamp:  1000000000,
				SourceId:   "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
				InstanceId: "4",
				Tags: map[string]string{
					"origin":     "test-origin",
					"deployment": "deployment-name",
					"job":        "doppler",
					"index":      "1",
					"ip":         "10.0.1.2",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit:  "gauge",
								Value: float64(1),
							},
						},
					},
				},
			}

			metrics, err := a.Parse(event)

			Expect(err).To(BeNil())
			Expect(metrics).To(HaveLen(10))

			for _, metric := range metrics {
				Expect(metric.MetricValue.Tags).To(ContainElement("app_name:hello-datadog-cf-ruby-dev"))
				Expect(metric.MetricValue.Tags).To(ContainElement("guid:6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
				Expect(metric.MetricValue.Tags).To(ContainElement("custom:tag"))
				Expect(metric.MetricValue.Tags).To(ContainElement("foo:bar"))
				Expect(metric.MetricValue.Tags).To(ContainElement("env:env_name"))
			}
		})
	})
})

type containMetric struct {
	needle   string
	haystack []metric.MetricPackage
}

func ContainMetric(name string) types.GomegaMatcher {
	return &containMetric{
		needle: name,
	}
}

func (m *containMetric) Match(actual interface{}) (success bool, err error) {
	var ok bool
	m.haystack, ok = actual.([]metric.MetricPackage)
	if !ok {
		return false, errors.New("actual must be of type []metrics.MetricPackage")
	}
	for _, pkg := range m.haystack {
		if pkg.MetricKey.Name == m.needle {
			return true, nil
		}
	}
	return false, nil
}

func (m *containMetric) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %#v to contain a metric named %s", m.haystack, m.needle)
}

func (m *containMetric) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Did not expect %#v to contain a metric named %s", m.haystack, m.needle)
}

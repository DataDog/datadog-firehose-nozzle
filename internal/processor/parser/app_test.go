package parser

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo/v2"
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
		fakeClusterAgentAPI    *FakeClusterAgentAPI
		dcaAPIURL              string
		fakeDCAClient          *cloudfoundry.DCAClient
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("datadogclient test")

		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()
		ccAPIURL = fakeCloudControllerAPI.URL()

		fakeClusterAgentAPI = NewFakeClusterAgentAPI("bearer", "123456789")
		fakeClusterAgentAPI.Start()
		dcaAPIURL = fakeClusterAgentAPI.URL()

		config.NozzleConfig = config.Config{}
		cfg := config.Config{
			CloudControllerEndpoint: ccAPIURL,
			Client:                  "bearer",
			ClientSecret:            "123456789",
			InsecureSSLSkipVerify:   true,
			NumWorkers:              5,
			DCAUrl:                  dcaAPIURL,
			DCAToken:                "123456789",
			DCAEnabled:              true,
			EnableAdvancedTagging:   true,
		}
		var err error
		fakeCfClient, err = cloudfoundry.NewClient(&cfg, log)
		Expect(err).ToNot(HaveOccurred())
		fakeDCAClient, err = cloudfoundry.NewDCAClient(&cfg, log)
		Expect(err).ToNot(HaveOccurred())
	}, 0.0)

	Context("generator function", func() {
		It("errors out properly when it cannot connect", func() {
			_, err := NewAppParser(nil, nil, 5, 10, log, []string{}, "", false)
			Expect(err).To(HaveOccurred())
		})

		It("generates it properly when it can connect using cloudfoundry client", func() {
			a, err := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			a.Stop()
			Eventually(a.Done()).Should(BeClosed())
		})

		It("generates it properly when it can connect using cluster agent client", func() {
			a, err := NewAppParser(nil, fakeDCAClient, 5, 10, log, []string{}, "", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			a.Stop()
			Eventually(a.Done()).Should(BeClosed())
		})
	})

	Context("cache warmup", func() {
		It("requests all the apps directly at startup using cluster agent client when when cluster agent is enabled", func() {
			a, err := NewAppParser(nil, fakeDCAClient, 5, 999, log, []string{}, "", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			Expect(a.AppCache.apps).To(HaveLen(21))
		})

		It("requests all the apps directly at startup using cloudfoundry client only when cluster agent is disabled", func() {
			a, err := NewAppParser(fakeCfClient, nil, 5, 999, log, []string{}, "", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			Expect(a.AppCache.apps).To(HaveLen(14))
		})

		It("does not block while warming cache using cloudfoundry client", func() {
			fakeCloudControllerAPI.RequestTime = 100
			a, err := NewAppParser(fakeCfClient, nil, 5, 999, log, []string{}, "", false)
			// Assertions are done while cache is warming up in the background
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.AppCache.IsWarmedUp()).To(BeFalse())
			// Eventually, the cache is ready
			Eventually(a.AppCache.IsWarmedUp, 10*time.Second).Should(BeTrue())
		})

		It("does not block while warming cache using cluster agent client", func() {
			fakeClusterAgentAPI.RequestTime = 100
			a, err := NewAppParser(nil, fakeDCAClient, 5, 999, log, []string{}, "", false)
			// Assertions are done while cache is warming up in the background
			Expect(err).ToNot(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.AppCache.IsWarmedUp()).To(BeFalse())
			// Eventually, the cache is ready
			Eventually(a.AppCache.IsWarmedUp, 10*time.Second).Should(BeTrue())
		})
	})

	Context("app metrics test", func() {
		It("tries to get it from the cloud controller when not in the cache and cluster agent is disabled", func() {
			a, _ := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "", false)
			var req *http.Request
			// Wait for cache warmup to finish
			Eventually(fakeCloudControllerAPI.ReceivedRequests).ShouldNot(Receive())
			_, err := a.getAppData("app-5")
			Expect(err).To(HaveOccurred()) // error expected because fake CC won't return an app, so unmarshalling will fail
			Eventually(fakeCloudControllerAPI.ReceivedRequests).Should(Receive(&req))
			Expect(req.URL.Path).To(Equal("/v2/apps/app-5"))
		})

		It("tries to get it from the cluster agent api when not in the cache", func() {
			a, _ := NewAppParser(nil, fakeDCAClient, 5, 10, log, []string{}, "", false)
			var req *http.Request
			// Wait for cache warmup to finish
			Eventually(fakeClusterAgentAPI.ReceivedRequests).ShouldNot(Receive())
			_, err := a.getAppData("app-5")
			Expect(err).To(HaveOccurred()) // error expected because fake API won't return an app, so unmarshalling will fail
			Eventually(fakeClusterAgentAPI.ReceivedRequests).Should(Receive(&req))
			Expect(req.URL.Path).To(Equal("/api/v1/cf/apps/app-5"))
		})

		It("grabs from the cache when it present using cloudfoundry client", func() {
			a, _ := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "", false)
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			// 6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a corresponds to hello-datadog-cf-ruby-dev
			Expect(a.AppCache.apps).To(HaveKey("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
			app, err := a.getAppData("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a")
			Expect(err).ToNot(HaveOccurred())
			Expect(app).NotTo(BeNil())
		})

		It("grabs from the cache when it present using cluster agent client", func() {
			a, _ := NewAppParser(nil, fakeDCAClient, 5, 10, log, []string{}, "", false)
			Eventually(a.AppCache.IsWarmedUp).Should(BeTrue())
			// 6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a corresponds to hello-datadog-cf-ruby-dev
			Expect(a.AppCache.apps).To(HaveKey("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"))
			app, err := a.getAppData("6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a")
			Expect(err).ToNot(HaveOccurred())
			Expect(app).NotTo(BeNil())
		})
	})

	Context("metric evaluation test", func() {
		It("parses an event properly and adds metadata if configured using cloudfoundry client", func() {
			config.NozzleConfig.EnableMetadataCollection = true
			config.NozzleConfig.EnableMetadataAppMetricsPrefix = true
			config.NozzleConfig.MetadataKeysBlacklist = []*regexp.Regexp{regexp.MustCompile("blacklisted.*")}
			a, err := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "env_name", false)
			Expect(err).ToNot(HaveOccurred())
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

			Expect(err).ToNot(HaveOccurred())
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
				Expect(metric.MetricValue.Tags).To(ContainElement("label/app-space-org-label:app-space-org-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/app-space-label:app-space-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/app-org-label:app-org-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/app-label:app-label-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/space-org-label:space-org-label-space-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/space-label:space-label-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("label/org-label:org-label-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/app-space-org-annotation:app-space-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/app-space-annotation:app-space-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/app-org-annotation:app-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/app-annotation:app-annotation-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/space-org-annotation:space-org-annotation-space-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/space-annotation:space-annotation-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("annotation/org-annotation:org-annotation-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-annotation-tag:auto-annotation-tag-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-label-tag:auto-label-tag-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("sidecar_present:true"))
				Expect(metric.MetricValue.Tags).To(ContainElement("sidecar_count:1"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:foo"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:bar"))

			}
		})

		It("parses an event properly and adds metadata if configured using cluster agent client", func() {
			config.NozzleConfig.EnableMetadataCollection = true
			config.NozzleConfig.MetadataKeysBlacklist = []*regexp.Regexp{regexp.MustCompile("blacklisted.*")}
			a, err := NewAppParser(nil, fakeDCAClient, 5, 10, log, []string{}, "env_name", false)
			Expect(err).ToNot(HaveOccurred())
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

			Expect(err).ToNot(HaveOccurred())
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
				Expect(metric.MetricValue.Tags).To(ContainElement("app-space-org-label:app-space-org-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-space-label:app-space-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-org-label:app-org-label-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-label:app-label-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-space-org-annotation:app-space-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-space-annotation:app-space-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-org-annotation:app-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("app-annotation:app-annotation-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-annotation-tag:auto-annotation-tag-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-label-tag:auto-label-tag-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("sidecar_present:true"))
				Expect(metric.MetricValue.Tags).To(ContainElement("sidecar_count:1"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:foo"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:bar"))
			}
		})

		It("parses an event properly and doesn't add metadata if not configured, except for autodiscovery tags", func() {
			a, err := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "env_name", false)
			Expect(err).ToNot(HaveOccurred())
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

			Expect(err).ToNot(HaveOccurred())
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
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-annotation-tag:auto-annotation-tag-value"))
				Expect(metric.MetricValue.Tags).To(ContainElement("auto-label-tag:auto-label-tag-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-space-org-label:app-space-org-label-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-space-label:app-space-label-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-org-label:app-org-label-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-label:app-label-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("space-org-label:space-org-label-space-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("space-label:space-label-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("org-label:org-label-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-space-org-annotation:app-space-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-space-annotation:app-space-annotation-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-org-annotation:app-org-annotation-app-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("app-annotation:app-annotation-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("space-org-annotation:space-org-annotation-space-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("space-annotation:space-annotation-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("org-annotation:org-annotation-value"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("whitelisted_key:foo"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("whitelisted_key:bar"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:foo"))
				Expect(metric.MetricValue.Tags).ToNot(ContainElement("blacklisted_key:bar"))
			}
		})
	})

	Context("expected tags", func() {
		It("adds proper instance tag", func() {
			a, err := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{}, "env_name", false)
			Expect(err).ToNot(HaveOccurred())
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
			a, err := NewAppParser(fakeCfClient, nil, 5, 10, log, []string{"custom:tag", "foo:bar"},
				"env_name", false)
			Expect(err).ToNot(HaveOccurred())
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

			Expect(err).ToNot(HaveOccurred())
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

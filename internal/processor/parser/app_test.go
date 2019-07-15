package parser

import (
	"errors"
	"fmt"

	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	cfclient "github.com/cloudfoundry-community/go-cfclient"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

var _ = Describe("AppMetrics", func() {
	var (
		log                    *gosteno.Logger
		fakeCloudControllerAPI *FakeCloudControllerAPI
		ccAPIURL               string
		fakeCfClient           *cfclient.Client
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("datadogclient test")
		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()

		ccAPIURL = fakeCloudControllerAPI.URL()
		cfg := cfclient.Config{
			ApiAddress:        ccAPIURL,
			ClientID:          "bearer",
			ClientSecret:      "123456789",
			SkipSslValidation: true,
			UserAgent:         "datadog-firehose-nozzle",
		}
		fakeCfClient, _ = cfclient.NewClient(&cfg)
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
			Expect(len(a.AppCache.apps)).To(Equal(4))
			for i := 1; i <= 4; i++ {
				Expect(a.AppCache.apps).To(HaveKey(fmt.Sprintf("app-%d", i)))
			}
		})
	})

	Context("app metrics test", func() {
		It("tries to get it from the cloud controller when the cache is empty", func() {
			a, _ := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "")
			_, err := a.getAppData("guid")
			Expect(err).NotTo(BeNil())
		})

		It("grabs from the cache when it should be", func() {
			a, _ := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "")
			guids := []string{"guid1", "guid2"}
			a.AppCache = *newFakeApps(guids)
			app, err := a.getAppData("guid1")
			Expect(err).To(BeNil())
			Expect(app).NotTo(BeNil())
		})
	})

	Context("metric evaluation test", func() {
		It("parses an event properly", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{}, "env_name")
			Expect(err).To(BeNil())
			guids := []string{"guid1", "guid2"}
			a.AppCache = *newFakeApps(guids)

			event := &events.Envelope{
				Origin:    proto.String("test-origin"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_ContainerMetric.Enum(),

				ContainerMetric: &events.ContainerMetric{
					CpuPercentage:    proto.Float64(float64(1)),
					DiskBytes:        proto.Uint64(uint64(1)),
					DiskBytesQuota:   proto.Uint64(uint64(1)),
					MemoryBytes:      proto.Uint64(uint64(1)),
					MemoryBytesQuota: proto.Uint64(uint64(1)),
					ApplicationId:    proto.String("guid1"),
				},

				// fields that gets sent as tags
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
				Index:      proto.String("1"),
				Ip:         proto.String("10.0.1.2"),
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
				Expect(metric.MetricValue.Tags).To(ContainElement("app_name:guid1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("guid:guid1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("env:env_name"))
			}
		})
	})

	Context("custom tags", func() {
		It("attaches custom tags if present", func() {
			a, err := NewAppParser(fakeCfClient, 5, 10, log, []string{"custom:tag", "foo:bar"}, "env_name")
			Expect(err).To(BeNil())
			guids := []string{"guid1", "guid2"}
			a.AppCache = *newFakeApps(guids)

			event := &events.Envelope{
				Origin:    proto.String("test-origin"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_ContainerMetric.Enum(),

				ContainerMetric: &events.ContainerMetric{
					CpuPercentage:    proto.Float64(float64(1)),
					DiskBytes:        proto.Uint64(uint64(1)),
					DiskBytesQuota:   proto.Uint64(uint64(1)),
					MemoryBytes:      proto.Uint64(uint64(1)),
					MemoryBytesQuota: proto.Uint64(uint64(1)),
					ApplicationId:    proto.String("guid1"),
				},

				// fields that gets sent as tags
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
				Index:      proto.String("1"),
				Ip:         proto.String("10.0.1.2"),
			}

			metrics, err := a.Parse(event)

			Expect(err).To(BeNil())
			Expect(metrics).To(HaveLen(10))

			for _, metric := range metrics {
				Expect(metric.MetricValue.Tags).To(ContainElement("app_name:guid1"))
				Expect(metric.MetricValue.Tags).To(ContainElement("guid:guid1"))
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
		return false, errors.New("Actual must be of type []metrics.MetricPackage")
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

func newFakeApps(guids []string) *appCache {
	cache := newAppCache()
	for _, guid := range guids {
		cache.apps[guid] = &App{
			Name:                   guid,
			GUID:                   guid,
			TotalDiskConfigured:    1,
			TotalMemoryConfigured:  1,
			TotalDiskProvisioned:   1,
			TotalMemoryProvisioned: 1,
		}
	}

	return &cache
}

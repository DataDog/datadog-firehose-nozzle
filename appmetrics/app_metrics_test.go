package appmetrics

import (
	"time"

	. "github.com/DataDog/datadog-firehose-nozzle/testhelpers"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

var _ = Describe("AppMetrics", func() {
	var (
		log                    *gosteno.Logger
		fakeCloudControllerAPI *FakeCloudControllerAPI
		ccAPIURL               string
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("datadogclient test")
		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()
		ccAPIURL = fakeCloudControllerAPI.URL()
	}, 0)

	Context("genertor function", func() {
		It("errors out properly when it cannot connect", func() {
			_, err := New("http://localhost", "", "", true, 10, log)
			Expect(err).NotTo(BeNil())
		})

		It("generates it properly when it can connect", func() {
			a, err := New(ccAPIURL, "bearer", "123456789", true, 10, log)
			Expect(err).To(BeNil())
			Expect(a).NotTo(BeNil())
		})
	})

	Context("app metrics test", func() {
		It("tries to get it from the cloud controller when the cache is empty", func() {
			a, _ := New(ccAPIURL, "bearer", "123456789", true, 10, log)
			_, err := a.getAppData("guid")
			Expect(err).NotTo(BeNil())
		})

		It("grabs from the cache when it should be", func() {
			a, _ := New(ccAPIURL, "bearer", "123456789", true, 10, log)
			guids := []string{"guid1", "guid2"}
			a.Apps = newFakeApps(guids)
			app, err := a.getAppData("guid1")
			Expect(err).To(BeNil())
			Expect(app).NotTo(BeNil())
		})
	})

	Context("metric evaluation test", func() {
		It("parses an event properly", func() {
			a, err := New(ccAPIURL, "bearer", "123456789", true, 10, log)
			Expect(err).To(BeNil())
			guids := []string{"guid1", "guid2"}
			a.Apps = newFakeApps(guids)

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

			metrics, err := a.ParseAppMetric(event)

			Expect(err).To(BeNil())
			Expect(metrics).To(HaveLen(10))
			Expect(metrics[0].MetricValue.Tags).To(HaveLen(2))
		})
	})

})

func newFakeApps(guids []string) map[string]*App {
	apps := map[string]*App{}
	for _, guid := range guids {
		apps[guid] = &App{
			Name:                   guid,
			GUID:                   guid,
			updated:                time.Now().Unix(),
			ErrorGrabbing:          false,
			TotalDiskConfigured:    1,
			TotalMemoryConfigured:  1,
			TotalDiskProvisioned:   1,
			TotalMemoryProvisioned: 1,
			Instances:              map[int32]Instance{},
		}
	}

	return apps
}

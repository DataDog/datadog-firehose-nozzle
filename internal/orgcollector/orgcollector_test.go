package orgcollector

import (
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/gosteno"
	//"github.com/cloudfoundry-community/go-cfclient"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
)

var _ = Describe("OrgCollector", func() {
	var (
		log                    *gosteno.Logger
		fakeCloudControllerAPI *FakeCloudControllerAPI
		ccAPIURL               string
		fakeOrgCollector       *OrgCollector
		pm                     chan []metric.MetricPackage
		customTags             []string
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("cloudfoundry client test")
		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()

		ccAPIURL = fakeCloudControllerAPI.URL()
		cfg := config.Config{
			CloudControllerEndpoint: ccAPIURL,
			Client:                  "bearer",
			ClientSecret:            "123456789",
			InsecureSSLSkipVerify:   true,
			NumWorkers:              0,
		}
		pm = make(chan []metric.MetricPackage, 1)
		customTags = []string{"foo:bar"}

		var err error
		fakeOrgCollector, err = NewOrgCollector(
			&cfg,
			pm,
			log,
			customTags,
		)
		Expect(err).To(BeNil())
	}, 0)

	It("pushes correct metrics", func() {
		fakeOrgCollector.pushMetrics()
		pushed := <-pm

		k1 := pushed[0].MetricKey
		v1 := pushed[0].MetricValue
		Expect(k1.Name).To(Equal("org.memory.quota"))
		Expect(v1.Tags).To(Equal([]string{
			"foo:bar",
			"guid:671557cf-edcd-49df-9863-ee14513d13c7",
			"org_id:671557cf-edcd-49df-9863-ee14513d13c7",
			"org_name:system",
			"status:active",
		}))
		Expect(v1.Points).To(HaveLen(1))
		Expect(v1.Points[0].Timestamp).To(BeNumerically(">", 0))
		Expect(v1.Points[0].Value).To(Equal(float64(102400)))

		k2 := pushed[1].MetricKey
		v2 := pushed[1].MetricValue
		Expect(k2.Name).To(Equal("org.memory.quota"))
		Expect(v2.Tags).To(Equal([]string{
			"foo:bar",
			"guid:8c19a50e-7974-4c67-adea-9640fae21526",
			"org_id:8c19a50e-7974-4c67-adea-9640fae21526",
			"org_name:datadog-application-monitoring-org",
			"status:active",
		}))
		Expect(v2.Points).To(HaveLen(1))
		Expect(v2.Points[0].Timestamp).To(BeNumerically(">", 0))
		Expect(v2.Points[0].Value).To(Equal(float64(102400)))
	})
})

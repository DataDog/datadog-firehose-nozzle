package cloudfoundry

import (
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	"github.com/cloudfoundry-community/go-cfclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry/gosteno"
)

func checkDCAAppAttributes(app *CFApplication) {
	Expect(app.GUID).To(Equal("a7bebd67-1991-4e9e-8d44-399acf2f13e8"))
	Expect(app.Name).To(Equal("logs-backend-demo"))
	Expect(app.SpaceGUID).To(Equal("68a6159c-cb4a-4f70-bb48-f2c24bd79c6b"))
	Expect(app.SpaceName).To(Equal("system"))
	Expect(app.OrgGUID).To(Equal("24d7098c-832b-4dfa-a4f1-950780ae92e9"))
	Expect(app.OrgName).To(Equal("system"))
	Expect(app.Instances).To(Equal(2))
	Expect(app.Buildpacks).To(Equal([]string{"datadog_application_monitoring", "ruby_buildpack"}))
	Expect(app.DiskQuota).To(Equal(1024))
	Expect(app.TotalDiskQuota).To(Equal(2048))
	Expect(app.Memory).To(Equal(256))
	Expect(app.TotalMemory).To(Equal(512))
	Expect(app.Annotations).To(Equal(map[string]string{
		"app-space-org-annotation": "app-space-org-annotation-app-value",
		"app-space-annotation":     "app-space-annotation-app-value",
		"app-org-annotation":       "app-org-annotation-app-value",
		"app-annotation":           "app-annotation-value",
		"space-org-annotation":     "space-org-annotation-space-value",
		"space-annotation":         "space-annotation-value",
		"org-annotation":           "org-annotation-value",
	}))
	Expect(app.Labels).To(Equal(map[string]string{
		"app-space-org-label": "app-space-org-label-app-value",
		"app-space-label":     "app-space-label-app-value",
		"app-org-label":       "app-org-label-app-value",
		"app-label":           "app-label-value",
		"space-org-label":     "space-org-label-space-value",
		"space-label":         "space-label-value",
		"org-label":           "org-label-value",
	}))
}

func checkVersionAttributes(version *Version) {
	Expect(version.Major).To(BeEquivalentTo(1))
	Expect(version.Minor).To(BeEquivalentTo(9))
	Expect(version.Patch).To(BeEquivalentTo(5))
	Expect(version.Pre).To(Equal("rc100"))
	Expect(version.Meta).To(Equal("git.2598.2b50b49"))
	Expect(version.Commit).To(Equal("2b50b49"))
}

func checkV3OrgAttributes(org *cfclient.V3Organization) {
	Expect(org.Name).To(Equal("system"))
	Expect(org.GUID).To(Equal("24d7098c-832b-4dfa-a4f1-950780ae92e9"))
	Expect(org.Relationships["quota"].Data.GUID).To(Equal("9f3f17e5-dabe-49b9-82c6-aa9e79724bdd"))
}

func checkV2OrgQuotaAttributes(orgQuota *CFOrgQuota) {
	Expect(orgQuota.GUID).To(Equal("9f3f17e5-dabe-49b9-82c6-aa9e79724bdd"))
	Expect(orgQuota.MemoryLimit).To(Equal(10240))
}

var _ = Describe("DatadogClusterAgentClient", func() {
	var (
		log                 *gosteno.Logger
		fakeClusterAgentAPI *FakeClusterAgentAPI
		dcaAPIURL           string
		fakeDCAClient       *DCAClient
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("DCA client test")
		fakeClusterAgentAPI = NewFakeClusterAgentAPI("bearer", "123456789")
		fakeClusterAgentAPI.Start()

		dcaAPIURL = fakeClusterAgentAPI.URL()
		cfg := config.Config{
			DCAUrl:     dcaAPIURL,
			DCAToken:   "123456789",
			DCAEnabled: true,
		}
		var err error
		fakeDCAClient, err = NewDCAClient(&cfg, log)
		Expect(err).To(BeNil())
	}, 0)

	Context("GetVersion method", func() {
		It("retrieves the cluster agent api version correctly", func() {
			res, err := fakeDCAClient.GetVersion()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkVersionAttributes(&res)
		})
	})

	Context("GetApplications method", func() {
		It("retrieves all apps correctly from the cluster agent api", func() {
			res, err := fakeDCAClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(21))
			checkDCAAppAttributes(&res[0])
		})
	})

	Context("GetApplication method", func() {
		It("retrieves a single app correctly from the cluster agent api", func() {
			res, err := fakeDCAClient.GetApplication("a7bebd67-1991-4e9e-8d44-399acf2f13e8")
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkDCAAppAttributes(res)
		})
	})

	Context("GetV3Orgs method", func() {
		It("retrieves all orgs correctly from the cluster agent api", func() {
			res, err := fakeDCAClient.GetV3Orgs()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkV3OrgAttributes(&res[0])
		})
	})

	Context("GetV2OrgQuotas method", func() {
		It("retrieves all orgs correctly from the cluster agent api", func() {
			res, err := fakeDCAClient.GetV2OrgQuotas()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkV2OrgQuotaAttributes(&res[0])
		})
	})
})

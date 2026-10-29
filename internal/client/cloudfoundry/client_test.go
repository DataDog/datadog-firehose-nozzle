package cloudfoundry

import (
	"net/http"

	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/cloudfoundry/gosteno"
)

func checkAppAttributes(app *CFApplication, advancedTagging bool) {
	Expect(app.GUID).To(Equal("6d254438-cc3b-44a6-b2e6-343ca92deb5f"))
	Expect(app.Name).To(Equal("p-invitations-green"))
	Expect(app.SpaceGUID).To(Equal("417b893e-291e-48ec-94c7-7b2348604365"))
	Expect(app.SpaceName).To(Equal("system"))
	Expect(app.OrgGUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
	Expect(app.OrgName).To(Equal("system"))
	Expect(app.Instances).To(Equal(1))
	Expect(app.Buildpacks).To(Equal([]string{"nodejs_buildpack"}))
	Expect(app.DiskQuota).To(Equal(1024))
	Expect(app.TotalDiskQuota).To(Equal(1024))
	Expect(app.Memory).To(Equal(256))
	Expect(app.TotalMemory).To(Equal(256))
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
	if advancedTagging {
		Expect(app.Sidecars).NotTo(BeNil())
		Expect(app.Sidecars).To(HaveLen(1))
		Expect(app.Sidecars[0].GUID).To(Equal("68a03f42-5392-47ed-9979-477d48a61927"))
		Expect(app.Sidecars[0].Name).To(Equal("config-server"))
	} else {
		Expect(app.Sidecars).To(BeNil())
	}
}

func checkProcessAttributes(process *resource.Process) {
	Expect(process.GUID).To(Equal("6d254438-cc3b-44a6-b2e6-343ca92deb5f"))
	Expect(process.Instances).To(Equal(1))
	Expect(process.DiskInMB).To(Equal(1024))
	Expect(process.MemoryInMB).To(Equal(256))
}

func checkSpaceAttributes(space *resource.Space) {
	Expect(space.GUID).To(Equal("417b893e-291e-48ec-94c7-7b2348604365"))
	Expect(space.Name).To(Equal("system"))
	Expect(space.Relationships.Organization.Data.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
}

func checkOrgAttributes(org *resource.Organization) {
	Expect(org.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
	Expect(org.Name).To(Equal("system"))
	Expect(metadataToMap(org.Metadata.Annotations)).To(Equal(map[string]string{
		"org-annotation":           "org-annotation-value",
		"app-org-annotation":       "app-org-annotation-org-value",
		"space-org-annotation":     "space-org-annotation-org-value",
		"app-space-org-annotation": "app-space-org-annotation-org-value",
	}))
	Expect(metadataToMap(org.Metadata.Labels)).To(Equal(map[string]string{
		"org-label":           "org-label-value",
		"app-org-label":       "app-org-label-org-value",
		"space-org-label":     "space-org-label-org-value",
		"app-space-org-label": "app-space-org-label-org-value",
	}))
	Expect(org.Relationships.Quota.Data.GUID).To(Equal("1cf98856-aba8-49a8-8b21-d82a25898c4e"))
}

var _ = Describe("CloudFoundryClient", func() {
	var (
		log                    *gosteno.Logger
		fakeCloudControllerAPI *FakeCloudControllerAPI
		ccAPIURL               string
		fakeCfClient           *CFClient
	)

	BeforeEach(func() {
		log = gosteno.NewLogger("cloudfoundry client test")
		fakeCloudControllerAPI = NewFakeCloudControllerAPI("bearer", "123456789")
		fakeCloudControllerAPI.Start()

		ccAPIURL = fakeCloudControllerAPI.URL()
		cfg := config.Config{
			CloudControllerEndpoint:     ccAPIURL,
			Client:                      "bearer",
			ClientSecret:                "123456789",
			InsecureSSLSkipVerify:       true,
			CloudControllerAPIBatchSize: 500,
		}
		var err error
		fakeCfClient, err = NewClient(&cfg, log)
		Expect(err).ToNot(HaveOccurred())
	}, 0.0)

	AfterEach(func() {
		fakeCloudControllerAPI.Close()
	})

	Context("NewClient", func() {
		It("discovers the auth endpoints from the v3 API root, not /v2/info", func() {
			Expect(fakeCloudControllerAPI.GetUsedEndpoints()).To(ContainElement("/"))
			Expect(fakeCloudControllerAPI.GetUsedEndpoints()).NotTo(ContainElement("/v2/info"))
		})
	})

	Context("individual endpoint", func() {
		It("with v3 spaces is retrieved correctly", func() {
			res, err := fakeCfClient.getSpaces()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(6))
			checkSpaceAttributes(res[0])
		})

		It("with v3 processes is retrieved correctly", func() {
			res, err := fakeCfClient.getProcesses()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(19))
			checkProcessAttributes(res[0])
		})

		It("with v3 orgs is retrieved correctly", func() {
			res, err := fakeCfClient.GetOrgs()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(2))
			checkOrgAttributes(res[0])
		})

		It("with v3 apps is retrieved correctly", func() {
			res, err := fakeCfClient.getApps()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(14))
		})

		It("sends the configured batch size as per_page", func() {
			_, err := fakeCfClient.getApps()
			Expect(err).ToNot(HaveOccurred())
			var req *http.Request
			Eventually(fakeCloudControllerAPI.ReceivedRequests).Should(Receive(&req, WithTransform(func(r *http.Request) string {
				return r.URL.Path
			}, Equal("/v3/apps"))))
			Expect(req.URL.Query().Get("per_page")).To(Equal("500"))
		})
	})

	Context("GetApplications method", func() {
		It("retrieves apps correctly without advanced tags", func() {
			res, err := fakeCfClient.GetApplications()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(14))
			checkAppAttributes(&res[0], fakeCfClient.advancedTagging)
		})

		It("retrieves apps correctly with advanced tags", func() {
			fakeCfClient.advancedTagging = true
			res, err := fakeCfClient.GetApplications()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(HaveLen(14))
			checkAppAttributes(&res[0], fakeCfClient.advancedTagging)
		})

		It("leaves Sidecars nil for apps without sidecars with advanced tags", func() {
			fakeCfClient.advancedTagging = true
			res, err := fakeCfClient.GetApplications()
			Expect(err).ToNot(HaveOccurred())
			for _, app := range res {
				if app.GUID != "6d254438-cc3b-44a6-b2e6-343ca92deb5f" && app.GUID != "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a" {
					Expect(app.Sidecars).To(BeNil(), "app %s", app.GUID)
				}
			}
		})

		It("only queries v3 endpoints", func() {
			_, err := fakeCfClient.GetApplications()
			Expect(err).ToNot(HaveOccurred())
			for _, endpoint := range fakeCloudControllerAPI.GetUsedEndpoints() {
				Expect(endpoint).NotTo(HavePrefix("/v2/"))
			}
		})
	})

	Context("GetApplication method", func() {
		It("retrieves app correctly without advanced tags", func() {
			res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			checkAppAttributes(res, fakeCfClient.advancedTagging)
		})

		It("retrieves app correctly with advanced tags", func() {
			fakeCfClient.advancedTagging = true
			res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
			Expect(err).ToNot(HaveOccurred())
			Expect(res).NotTo(BeNil())
			checkAppAttributes(res, fakeCfClient.advancedTagging)
		})

		It("matches the app built by GetApplications", func() {
			apps, err := fakeCfClient.GetApplications()
			Expect(err).ToNot(HaveOccurred())
			res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
			Expect(err).ToNot(HaveOccurred())
			Expect(*res).To(Equal(apps[0]))
		})

		It("returns an error for an unknown app", func() {
			_, err := fakeCfClient.GetApplication("unknown-app")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetOrgQuotas method", func() {
		It("retrieves v3 organization quotas and maps unlimited memory to -1", func() {
			res, err := fakeCfClient.GetOrgQuotas()
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal([]CFOrgQuota{
				{GUID: "1345a873-ead6-4ae2-9d7d-e270c1a05c82", MemoryLimit: 10240},
				{GUID: "1cf98856-aba8-49a8-8b21-d82a25898c4e", MemoryLimit: 102400},
				{GUID: "5c1e3a3c-0b5e-4bd1-9e3a-6a3b1f6f0c11", MemoryLimit: -1},
			}))
			Expect(fakeCloudControllerAPI.GetUsedEndpoints()).To(ContainElement("/v3/organization_quotas"))
		})
	})
})

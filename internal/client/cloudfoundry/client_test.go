package cloudfoundry

import (
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
)

func checkAppAttributes(app *CFApplication, apiVersion int, advancedTagging bool) {
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
	if apiVersion == 3 {
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
	} else {
		Expect(app.Annotations).To(BeNil())
		Expect(app.Labels).To(BeNil())
	}
	if advancedTagging {
		Expect(app.Sidecars).NotTo(BeNil())
		Expect(len(app.Sidecars)).To(Equal(1))
		Expect(app.Sidecars[0].GUID).To(Equal("68a03f42-5392-47ed-9979-477d48a61927"))
		Expect(app.Sidecars[0].Name).To(Equal("config-server"))
	}
}

func checkProcessAttributes(process *cfclient.Process) {
	Expect(process.GUID).To(Equal("6d254438-cc3b-44a6-b2e6-343ca92deb5f"))
	Expect(process.Instances).To(Equal(1))
	Expect(process.DiskInMB).To(Equal(1024))
	Expect(process.MemoryInMB).To(Equal(256))
}

func checkSpaceAttributes(space *v3SpaceResource) {
	Expect(space.GUID).To(Equal("417b893e-291e-48ec-94c7-7b2348604365"))
	Expect(space.Name).To(Equal("system"))
	Expect(space.Relationships.Organization.Data.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
}

func checkOrgAttributes(org *v3OrgResource) {
	Expect(org.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
	Expect(org.Name).To(Equal("system"))
}

func checkV3OrgAttributes2(org *cfclient.V3Organization) {
	Expect(org.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
	Expect(org.Name).To(Equal("system"))
	Expect(org.Metadata.Annotations).To(Equal(map[string]string{
		"org-annotation":           "org-annotation-value",
		"app-org-annotation":       "app-org-annotation-org-value",
		"space-org-annotation":     "space-org-annotation-org-value",
		"app-space-org-annotation": "app-space-org-annotation-org-value",
	}))
	Expect(org.Metadata.Labels).To(Equal(map[string]string{
		"org-label":           "org-label-value",
		"app-org-label":       "app-org-label-org-value",
		"space-org-label":     "space-org-label-org-value",
		"app-space-org-label": "app-space-org-label-org-value",
	}))
	Expect(org.Relationships["quota"].Data.GUID).To(Equal("1cf98856-aba8-49a8-8b21-d82a25898c4e"))
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
			CloudControllerEndpoint: ccAPIURL,
			Client:                  "bearer",
			ClientSecret:            "123456789",
			InsecureSSLSkipVerify:   true,
			NumWorkers:              0,
		}
		var err error
		fakeCfClient, err = NewClient(&cfg, log)
		Expect(err).To(BeNil())
	}, 0.0)

	Context("individual endpoint", func() {
		It("with v2 apps is retrieved correctly", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)
		})

		It("with v3 spaces is retrieved correctly", func() {
			res, err := fakeCfClient.getV3Spaces()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(6))
			checkSpaceAttributes(&res[0])
		})

		It("with v3 processes is retrieved correctly", func() {
			res, err := fakeCfClient.getV3Processes()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(19))
			checkProcessAttributes(&res[0])
		})

		It("with v3 orgs is retrieved correctly", func() {
			res, err := fakeCfClient.getV3Orgs()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(2))
			checkOrgAttributes(&res[0])
		})

		It("with v3 apps is retrieved correctly", func() {
			res, err := fakeCfClient.getV3Apps()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
		})
	})

	Context("getV2Applications method", func() {
		It("retrieves apps correctly", func() {
			fakeCfClient.NumWorkers = 1
			res, err := fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)
		})
	})

	Context("getV3Applications method", func() {
		It("retrieves apps correctly without advanced tags", func() {
			res, err := fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
		})

		It("retrieves apps correctly with advanced tags", func() {
			fakeCfClient.advancedTagging = true
			res, err := fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
		})
	})

	Context("GetApplications method", func() {
		Context("without advanced tagging", func() {
			It("retrieves apps correctly without specified API Version", func() {
				fakeCfClient.NumWorkers = 1
				res, err := fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 100 // More runners than pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 2 // As many runners as pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
			})

			It("retrieves apps correctly with explicitly specified v3 API version", func() {
				fakeCfClient.ApiVersion = 3
				res, err := fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
			})

			It("retrieves apps correctly with explicitly specified v2 API version", func() {
				fakeCfClient.NumWorkers = 1
				fakeCfClient.ApiVersion = 2
				res, err := fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(len(res)).To(Equal(45))
				checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 100 // More runners than pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(len(res)).To(Equal(45))
				checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 3 // As many runners as pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(len(res)).To(Equal(45))
				checkAppAttributes(&res[0], 2, fakeCfClient.advancedTagging)
			})
		})

		Context("with advanced tagging", func() {
			BeforeEach(func() {
				fakeCfClient.advancedTagging = true
			})
			It("retrieves apps correctly without specified API Version", func() {
				fakeCfClient.NumWorkers = 1
				res, err := fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 100 // More runners than pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)

				fakeCfClient.NumWorkers = 2 // As many runners as pages
				res, err = fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(fakeCfClient.ApiVersion).To(Equal(3))
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
			})

			It("retrieves apps correctly with explicitly specified v3 API version", func() {
				fakeCfClient.ApiVersion = 3
				res, err := fakeCfClient.GetApplications()
				Expect(err).To(BeNil())
				Expect(res).NotTo(BeNil())
				Expect(len(res)).To(Equal(14))
				checkAppAttributes(&res[0], 3, fakeCfClient.advancedTagging)
			})
		})
	})

	Context("GetApplication method", func() {
		It("retrieves app correctly", func() {
			res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkAppAttributes(res, 2, fakeCfClient.advancedTagging)
		})
	})

	Context("GetV3Orgs  method", func() {
		It("retrieves v3 orgs correctly", func() {
			res, err := fakeCfClient.GetV3Orgs()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkV3OrgAttributes2(&res[0])
		})
	})
})

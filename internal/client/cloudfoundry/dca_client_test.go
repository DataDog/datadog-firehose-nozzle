package cloudfoundry

import (
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
)

func checkV3AppAttributes(app *cfclient.V3App, apiVersion int) {
	Expect(app.GUID).To(Equal("6d254438-cc3b-44a6-b2e6-343ca92deb5f"))
	Expect(app.Name).To(Equal("p-invitations-green"))
	Expect(app.Relationships["space"].Data.GUID).To(Equal("417b893e-291e-48ec-94c7-7b2348604365"))
	Expect(app.Metadata.Annotations).To(Equal(map[string]string{
		"app-space-org-annotation": "app-space-org-annotation-app-value",
		"app-space-annotation":     "app-space-annotation-app-value",
		"app-org-annotation":       "app-org-annotation-app-value",
		"app-annotation":           "app-annotation-value",
		"space-org-annotation":     "space-org-annotation-space-value",
		"space-annotation":         "space-annotation-value",
		"org-annotation":           "org-annotation-value",
	}))
	Expect(app.Metadata.Labels).To(Equal(map[string]string{
		"app-space-org-label": "app-space-org-label-app-value",
		"app-space-label":     "app-space-label-app-value",
		"app-org-label":       "app-org-label-app-value",
		"app-label":           "app-label-value",
		"space-org-label":     "space-org-label-space-value",
		"space-label":         "space-label-value",
		"org-label":           "org-label-value",
	}))
}

func checkV3ProcessAttributes(process *cfclient.Process) {
	Expect(process.GUID).To(Equal("6d254438-cc3b-44a6-b2e6-343ca92deb5f"))
	Expect(process.Instances).To(Equal(1))
	Expect(process.DiskInMB).To(Equal(1024))
	Expect(process.MemoryInMB).To(Equal(256))
}

func checkV3SpaceAttributes(space *cfclient.V3Space) {
	Expect(space.GUID).To(Equal("417b893e-291e-48ec-94c7-7b2348604365"))
	Expect(space.Name).To(Equal("system"))
	Expect(space.Relationships["organization"].Data.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
}

func checkV3OrgAttributes(org *cfclient.V3Organization) {
	Expect(org.GUID).To(Equal("671557cf-edcd-49df-9863-ee14513d13c7"))
	Expect(org.Name).To(Equal("system"))
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

	Context("individual endpoint", func() {
		It("with v3 spaces is retrieved correctly", func() {
			res, err := fakeDCAClient.GetCFSpaces()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(6))
			checkV3SpaceAttributes(&res[0])
		})

		It("with v3 processes is retrieved correctly", func() {
			res, err := fakeDCAClient.GetCFProcesses()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(19))
			checkV3ProcessAttributes(&res[0])
		})

		It("with v3 orgs is retrieved correctly", func() {
			res, err := fakeDCAClient.GetCFOrgs()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(2))
			checkV3OrgAttributes(&res[0])
		})

		It("with v3 apps is retrieved correctly", func() {
			res, err := fakeDCAClient.GetCFApps()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
		})
	})

	// Context("getV2Applications method", func() {
	// 	It("retrieves apps correctly", func() {
	// 		fakeCfClient.NumWorkers = 1
	// 		res, err := fakeCfClient.getV2Applications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)

	// 		fakeCfClient.NumWorkers = 100 // More runners than pages
	// 		res, err = fakeCfClient.getV2Applications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)

	// 		fakeCfClient.NumWorkers = 3 // As many runners as pages
	// 		res, err = fakeCfClient.getV2Applications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)
	// 	})
	// })

	// Context("getV3Applications method", func() {
	// 	It("retrieves apps correctly", func() {
	// 		res, err := fakeCfClient.getV3Applications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(14))
	// 		checkAppAttributes(&res[0], 3)
	// 	})
	// })

	// Context("GetApplications method", func() {
	// 	It("retrieves apps correctly without specified API Version", func() {
	// 		fakeCfClient.NumWorkers = 1
	// 		res, err := fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(fakeCfClient.ApiVersion).To(Equal(3))
	// 		Expect(len(res)).To(Equal(14))
	// 		checkAppAttributes(&res[0], 3)

	// 		fakeCfClient.NumWorkers = 100 // More runners than pages
	// 		res, err = fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(fakeCfClient.ApiVersion).To(Equal(3))
	// 		Expect(len(res)).To(Equal(14))
	// 		checkAppAttributes(&res[0], 3)

	// 		fakeCfClient.NumWorkers = 2 // As many runners as pages
	// 		res, err = fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(fakeCfClient.ApiVersion).To(Equal(3))
	// 		Expect(len(res)).To(Equal(14))
	// 		checkAppAttributes(&res[0], 3)
	// 	})

	// 	It("retrieves apps correctly with explicitly specified v3 API version", func() {
	// 		fakeCfClient.ApiVersion = 3
	// 		res, err := fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(14))
	// 		checkAppAttributes(&res[0], 3)
	// 	})

	// 	It("retrieves apps correctly with explicitly specified v2 API version", func() {
	// 		fakeCfClient.NumWorkers = 1
	// 		fakeCfClient.ApiVersion = 2
	// 		res, err := fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)

	// 		fakeCfClient.NumWorkers = 100 // More runners than pages
	// 		res, err = fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)

	// 		fakeCfClient.NumWorkers = 3 // As many runners as pages
	// 		res, err = fakeCfClient.GetApplications()
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		Expect(len(res)).To(Equal(45))
	// 		checkAppAttributes(&res[0], 2)
	// 	})
	// })

	// Context("GetApplication method", func() {
	// 	It("retrieves app correctly", func() {
	// 		res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
	// 		Expect(err).To(BeNil())
	// 		Expect(res).NotTo(BeNil())
	// 		checkAppAttributes(res, 2)
	// 	})
	// })
})

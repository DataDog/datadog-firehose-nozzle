package cloudfoundry

import (
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/gosteno"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
)

func checkAppAttributes(app *CFApplication) (error) {
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
	return nil
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
			CloudControllerEndpoint:	ccAPIURL,
			Client:          			"bearer",
			ClientSecret:      			"123456789",
			InsecureSSLSkipVerify: 		true,
			NumWorkers:					0,
		}
		var err error
		fakeCfClient, err = NewClient(&cfg, log)
		Expect(err).To(BeNil())
	}, 0)

	Context("individual endpoint", func() {
		It("with v2 apps is retrieved correctly", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
			checkAppAttributes(&res[0])
		})

		It("with v3 spaces is retrieved correctly", func() {
			res, err := fakeCfClient.getV3Spaces()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(6))
		})

		It("with v3 processes is retrieved correctly", func() {
			res, page, err := fakeCfClient.getV3ProcessesByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(2))
			Expect(len(res)).To(Equal(18))

			res, err = fakeCfClient.getV3Processes()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(19))
		})

		It("with v3 apps is retrieved correctly", func() {
			res, page, err := fakeCfClient.getV3AppsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(2))
			Expect(len(res)).To(Equal(13))

			res, page, err = fakeCfClient.getV3AppsByPage(2)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(2))
			Expect(len(res)).To(Equal(1))

			res, err = fakeCfClient.getV3Apps()
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
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0])
		})
	})

	Context("getV3Applications method", func() {
		It("retrieves apps correctly", func() {
			res, err := fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0])
		})
	})

	Context("GetApplications method", func() {
		It("retrieves apps correctly without specified API Version", func() {
			fakeCfClient.NumWorkers = 1
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 2 // As many runners as pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0])
		})

		It("retrieves apps correctly with explicitly specified v3 API version", func() {
			fakeCfClient.ApiVersion = 3
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(14))
			checkAppAttributes(&res[0])
		})

		It("retrieves apps correctly with explicitly specified v2 API version", func() {
			fakeCfClient.NumWorkers = 1
			fakeCfClient.ApiVersion = 2
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0])

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
			checkAppAttributes(&res[0])
		})
	})

	Context("GetApplication method", func() {
		It("retrieves app correctly", func() {
			res, err := fakeCfClient.GetApplication("6d254438-cc3b-44a6-b2e6-343ca92deb5f")
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			checkAppAttributes(res)
		})
	})
})


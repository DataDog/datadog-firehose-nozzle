package cloudfoundry

import (

	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/gosteno"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
)

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

	Context("individual endpoints", func() {
		It("v2 apps endpoint", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV2ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(15))
		})

		It("v3 space endpoint", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV3SpacesByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(6))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV3SpacesByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(6))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV3SpacesByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(6))
		})

		It("v3 processes endpoint", func() {
			res, err := fakeCfClient.getV3Processes()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(19))
		})

		It("v2 quota_definitions endpoint", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV2OrganizationsQuotasByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(3))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV2OrganizationsQuotasByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(3))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV2OrganizationsQuotasByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(3))
		})

		It("v3 apps endpoint", func() {
			fakeCfClient.NumWorkers = 1
			res, page, err := fakeCfClient.getV3ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(14))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, page, err = fakeCfClient.getV3ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(14))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, page, err = fakeCfClient.getV3ApplicationsByPage(1)
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(page).To(Equal(3))
			Expect(len(res)).To(Equal(14))
		})
	})

	Context("get Applications methods", func() {
		It("getV2Application", func() {
			fakeCfClient.NumWorkers = 1
			res, err := fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.getV2Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
		})

		It("getV3Application", func() {
			fakeCfClient.NumWorkers = 1
			res, err := fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.getV3Applications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))
		})

		It("GetApplications no API Version", func() {
			fakeCfClient.NumWorkers = 1
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(fakeCfClient.ApiVersion).To(Equal(3))
			Expect(len(res)).To(Equal(42))
		})

		It("GetApplications with v3 API Version", func() {
			fakeCfClient.NumWorkers = 1
			fakeCfClient.ApiVersion = 3
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(42))
		})

		It("GetApplications with v2 API Version", func() {
			fakeCfClient.NumWorkers = 1
			fakeCfClient.ApiVersion = 2
			res, err := fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))

			fakeCfClient.NumWorkers = 100 // More runners than pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))

			fakeCfClient.NumWorkers = 3 // As many runners as pages
			res, err = fakeCfClient.GetApplications()
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(len(res)).To(Equal(45))
		})
	})
})


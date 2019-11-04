package config

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NozzleConfig", func() {
	BeforeEach(func() {
		os.Clearenv()
	})

	It("successfully parses a valid config", func() {
		conf, err := Parse("testdata/test_config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut.cf-app.com"))
		Expect(conf.Client).To(Equal("user"))
		Expect(conf.ClientSecret).To(Equal("user_password"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq.com/api/v1/series"))
		Expect(conf.DataDogAPIKey).To(Equal("<enter api key>"))
		Expect(conf.HTTPProxyURL).To(Equal("http://user:password@host.com:port"))
		Expect(conf.HTTPSProxyURL).To(Equal("https://user:password@host.com:port"))
		Expect(conf.DataDogTimeoutSeconds).To(BeEquivalentTo(5))
		Expect(conf.FlushDurationSeconds).To(BeEquivalentTo(15))
		Expect(conf.FlushMaxBytes).To(BeEquivalentTo(57671680))
		Expect(conf.InsecureSSLSkipVerify).To(Equal(true))
		Expect(conf.MetricPrefix).To(Equal("datadogclient"))
		Expect(conf.Deployment).To(Equal("deployment-name"))
		Expect(conf.DeploymentFilter).To(Equal("deployment-filter"))
		Expect(conf.DisableAccessControl).To(Equal(false))
		Expect(conf.IdleTimeoutSeconds).To(BeEquivalentTo(60))
		Expect(conf.WorkerTimeoutSeconds).To(BeEquivalentTo(30))
		Expect(conf.CustomTags).To(BeEquivalentTo([]string{
			"nozzle:foobar",
			"env:prod",
			"role:db",
		}))
		Expect(conf.EnvironmentName).To(Equal("env_name"))
		Expect(conf.NumWorkers).To(Equal(1))
		Expect(conf.NumCacheWorkers).To(Equal(2))
		Expect(conf.GrabInterval).To(Equal(50))
		Expect(conf.CloudControllerAPIBatchSize).To(BeEquivalentTo(1000))
		Expect(conf.OrgDataQuerySeconds).To(BeEquivalentTo(100))
	})

	It("successfully sets default configuration values", func() {
		conf, err := Parse("testdata/test_config_defaults.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.MetricPrefix).To(Equal("cloudfoundry.nozzle."))
		Expect(conf.NumWorkers).To(BeEquivalentTo(4))
		Expect(conf.NumCacheWorkers).To(BeEquivalentTo(4))
		Expect(conf.IdleTimeoutSeconds).To(BeEquivalentTo(60))
		Expect(conf.WorkerTimeoutSeconds).To(BeEquivalentTo(10))
		Expect(conf.GrabInterval).To(Equal(10))
		Expect(conf.CloudControllerAPIBatchSize).To(BeEquivalentTo(500))
		Expect(conf.OrgDataQuerySeconds).To(BeEquivalentTo(600))
	})

	It("successfully overwrites file config values with environmental variables", func() {
		os.Setenv("NOZZLE_UAAURL", "https://uaa.walnut-env.cf-app.com")
		os.Setenv("NOZZLE_CLIENT", "env-user")
		os.Setenv("NOZZLE_CLIENT_SECRET", "env-user-password")
		os.Setenv("NOZZLE_DATADOGURL", "https://app.datadoghq-env.com/api/v1/series")
		os.Setenv("NOZZLE_DATADOGAPIKEY", "envapi-key>")
		os.Setenv("HTTP_PROXY", "http://test:proxy")
		os.Setenv("HTTPS_PROXY", "https://test:proxy")
		os.Setenv("NOZZLE_DATADOGTIMEOUTSECONDS", "10")
		os.Setenv("NOZZLE_FLUSHDURATIONSECONDS", "25")
		os.Setenv("NOZZLE_FLUSHMAXBYTES", "12345678")
		os.Setenv("NOZZLE_INSECURESSLSKIPVERIFY", "false")
		os.Setenv("NOZZLE_METRICPREFIX", "env-datadogclient")
		os.Setenv("NOZZLE_DEPLOYMENT", "env-deployment-name")
		os.Setenv("NOZZLE_DEPLOYMENT_FILTER", "env-deployment-filter")
		os.Setenv("NOZZLE_DISABLEACCESSCONTROL", "true")
		os.Setenv("NOZZLE_IDLETIMEOUTSECONDS", "30")
		os.Setenv("NOZZLE_WORKERTIMEOUTSECONDS", "20")
		os.Setenv("NO_PROXY", "google.com,datadoghq.com")
		os.Setenv("NOZZLE_ENVIRONMENT_NAME", "env_var_env_name")
		os.Setenv("NOZZLE_NUM_WORKERS", "3")
		os.Setenv("NOZZLE_NUM_CACHE_WORKERS", "5")
		os.Setenv("NOZZLE_GRAB_INTERVAL", "50")
		os.Setenv("NOZZLE_CLOUD_CONTROLLER_API_BATCH_SIZE", "100")
		os.Setenv("NOZZLE_ORG_DATA_QUERY_SECONDS", "100")
		conf, err := Parse("testdata/test_config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut-env.cf-app.com"))
		Expect(conf.Client).To(Equal("env-user"))
		Expect(conf.ClientSecret).To(Equal("env-user-password"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq-env.com/api/v1/series"))
		Expect(conf.DataDogAPIKey).To(Equal("envapi-key>"))
		Expect(conf.HTTPProxyURL).To(Equal("http://test:proxy"))
		Expect(conf.HTTPSProxyURL).To(Equal("https://test:proxy"))
		Expect(conf.NoProxy).To(BeEquivalentTo([]string{"google.com", "datadoghq.com"}))
		Expect(conf.DataDogTimeoutSeconds).To(BeEquivalentTo(10))
		Expect(conf.FlushDurationSeconds).To(BeEquivalentTo(25))
		Expect(conf.FlushMaxBytes).To(BeEquivalentTo(12345678))
		Expect(conf.InsecureSSLSkipVerify).To(Equal(false))
		Expect(conf.MetricPrefix).To(Equal("env-datadogclient"))
		Expect(conf.Deployment).To(Equal("env-deployment-name"))
		Expect(conf.DeploymentFilter).To(Equal("env-deployment-filter"))
		Expect(conf.DisableAccessControl).To(Equal(true))
		Expect(conf.WorkerTimeoutSeconds).To(BeEquivalentTo(20))
		Expect(conf.EnvironmentName).To(Equal("env_var_env_name"))
		Expect(conf.NumWorkers).To(Equal(3))
		Expect(conf.NumCacheWorkers).To(Equal(5))
		Expect(conf.GrabInterval).To(Equal(50))
		Expect(conf.CloudControllerAPIBatchSize).To(BeEquivalentTo(100))
		Expect(conf.OrgDataQuerySeconds).To(BeEquivalentTo(100))
	})

	It("correctly serializes to log string", func() {
		// For logs, we want this to be serialized as one long line without newlines
		expected := `{"AppMetrics":true,"Client":"user","ClientSecret":"*****","CloudControllerAPIBatchSize":1000,`
		expected += `"CloudControllerEndpoint":"string","CustomTags":["nozzle:foobar","env:prod","role:db"],`
		expected += `"DataDogAPIKey":"*****","DataDogAdditionalEndpoints":{"https://app.datadoghq.com/api/v1/series":["*****","*****"],`
		expected += `"https://app.datadoghq.com/api/v2/series":["*****"]},"DataDogTimeoutSeconds":5,`
		expected += `"DataDogURL":"https://app.datadoghq.com/api/v1/series","Deployment":"deployment-name",`
		expected += `"DeploymentFilter":"deployment-filter","DisableAccessControl":false,"EnvironmentName":"env_name",`
		expected += `"FirehoseSubscriptionID":"datadog-nozzle","FlushDurationSeconds":15,"FlushMaxBytes":57671680,`
		expected += `"GrabInterval":50,"HTTPProxyURL":"http://user:password@host.com:port",`
		expected += `"HTTPSProxyURL":"https://user:password@host.com:port","IdleTimeoutSeconds":60,"InsecureSSLSkipVerify":true,`
		expected += `"MetricPrefix":"datadogclient","NoProxy":[""],"NumCacheWorkers":2,"NumWorkers":1,`
		expected += `"OrgDataQuerySeconds":100,"TrafficControllerURL":"wss://doppler.walnut.cf-app.com:4443",`
		expected += `"UAAURL":"https://uaa.walnut.cf-app.com","WorkerTimeoutSeconds":30}`
		conf, err := Parse("testdata/test_config.json")
		Expect(err).ToNot(HaveOccurred())
		result, err := conf.AsLogString()
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(expected))
	})
})

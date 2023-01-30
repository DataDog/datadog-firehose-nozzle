package config

import (
	"os"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
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
		Expect(conf.RLPGatewayURL).To(Equal("https://some-url.blah"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq.com/api/v1/series"))
		Expect(conf.DataDogLogIntakeURL).To(Equal("https://http-intake.logs.datadoghq.com/api/v2/logs"))
		Expect(conf.DataDogAdditionalLogIntakeEndpoints).To(BeEquivalentTo([]string{"https://http-intake.logs.us3.datadoghq.com/api/v2/logs", "https://http-intake.logs.datadoghq.eu/api/v2/logs"}))
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
		Expect(conf.OrgDataCollectionInterval).To(BeEquivalentTo(100))
		Expect(conf.EnableMetadataCollection).To(BeTrue())
		Expect(conf.EnableApplicationLogs).To(BeTrue())
		Expect(conf.MetadataKeysBlacklist).To(BeEquivalentTo([]*regexp.Regexp{regexp.MustCompile("blacklisted1"), regexp.MustCompile("blacklisted2")}))
		Expect(conf.MetadataKeysWhitelist).To(BeEquivalentTo([]*regexp.Regexp{regexp.MustCompile("whitelisted1"), regexp.MustCompile("whitelisted2")}))
		Expect(conf.MetadataKeysBlacklistPatterns).To(BeEquivalentTo([]string{"blacklisted1", "blacklisted2"}))
		Expect(conf.MetadataKeysWhitelistPatterns).To(BeEquivalentTo([]string{"whitelisted1", "whitelisted2"}))
		Expect(conf.DCAEnabled).To(Equal(true))
		Expect(conf.DCAUrl).To(Equal("datadog-cluster-agent.bosh-deployment-name:5005"))
		Expect(conf.DCAToken).To(Equal("123456789"))
		Expect(conf.EnableAdvancedTagging).To(Equal(true))
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
		Expect(conf.OrgDataCollectionInterval).To(BeEquivalentTo(600))
		Expect(conf.EnableMetadataCollection).To(BeFalse())
		Expect(conf.MetadataKeysBlacklist).To(BeEmpty())
		Expect(conf.MetadataKeysWhitelist).To(BeEmpty())
		Expect(conf.MetadataKeysBlacklistPatterns).To(BeEmpty())
		Expect(conf.MetadataKeysWhitelistPatterns).To(BeEmpty())
		Expect(conf.DCAEnabled).To(BeFalse())
		Expect(conf.DCAUrl).To(BeEmpty())
		Expect(conf.DCAToken).To(BeEmpty())
		Expect(conf.EnableAdvancedTagging).To(BeFalse())
		Expect(conf.EnableApplicationLogs).To(BeFalse())
	})

	It("successfully overwrites file config values with environmental variables", func() {
		os.Setenv("NOZZLE_UAAURL", "https://uaa.walnut-env.cf-app.com")
		os.Setenv("NOZZLE_CLIENT", "env-user")
		os.Setenv("NOZZLE_CLIENT_SECRET", "env-user-password")
		os.Setenv("NOZZLE_RLP_GATEWAY_URL", "https://even-more-different.com")
		os.Setenv("NOZZLE_DATADOGURL", "https://app.datadoghq-env.com/api/v1/series")
		os.Setenv("NOZZLE_DATADOGLOGINTAKEURL", "https://http-intake.logs.datadoghq-env.com/api/v2/logs")
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
		os.Setenv("NOZZLE_ORG_DATA_COLLECTION_INTERVAL", "100")
		os.Setenv("NOZZLE_METADATA_KEYS_WHITELIST", "whitelisted1,whitelisted2")
		os.Setenv("NOZZLE_METADATA_KEYS_BLACKLIST", "blacklisted1,blacklisted2")
		os.Setenv("NOZZLE_ENABLE_METADATA_COLLECTION", "true")
		os.Setenv("NOZZLE_ENABLE_APPLICATION_LOGS", "true")
		os.Setenv("NOZZLE_DCA_ENABLED", "true")
		os.Setenv("NOZZLE_DCA_URL", "datadog-cluster-agent.bosh-deployment-name:5005")
		os.Setenv("NOZZLE_DCA_TOKEN", "123456789")
		os.Setenv("NOZZLE_DCA_ADVANCED_TAGGING", "true")
		conf, err := Parse("testdata/test_config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut-env.cf-app.com"))
		Expect(conf.Client).To(Equal("env-user"))
		Expect(conf.ClientSecret).To(Equal("env-user-password"))
		Expect(conf.RLPGatewayURL).To(Equal("https://even-more-different.com"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq-env.com/api/v1/series"))
		Expect(conf.DataDogLogIntakeURL).To(Equal("https://http-intake.logs.datadoghq-env.com/api/v2/logs"))
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
		Expect(conf.OrgDataCollectionInterval).To(BeEquivalentTo(100))
		Expect(conf.EnableMetadataCollection).To(BeTrue())
		Expect(conf.EnableApplicationLogs).To(BeTrue())
		Expect(conf.MetadataKeysBlacklist).To(BeEquivalentTo([]*regexp.Regexp{regexp.MustCompile("blacklisted1"), regexp.MustCompile("blacklisted2")}))
		Expect(conf.MetadataKeysWhitelist).To(BeEquivalentTo([]*regexp.Regexp{regexp.MustCompile("whitelisted1"), regexp.MustCompile("whitelisted2")}))
		Expect(conf.MetadataKeysBlacklistPatterns).To(BeEquivalentTo([]string{"blacklisted1", "blacklisted2"}))
		Expect(conf.DCAEnabled).To(BeEquivalentTo(true))
		Expect(conf.DCAUrl).To(Equal("datadog-cluster-agent.bosh-deployment-name:5005"))
		Expect(conf.DCAToken).To(Equal("123456789"))
		Expect(conf.EnableAdvancedTagging).To(BeEquivalentTo(true))
	})

	It("correctly serializes to log string", func() {
		// For logs, we want this to be serialized as one long line without newlines
		expected := `{"AppMetrics":true,"Client":"user","ClientSecret":"*****","CloudControllerAPIBatchSize":1000,`
		expected += `"CloudControllerEndpoint":"string","CustomTags":["nozzle:foobar","env:prod","role:db"],`
		expected += `"DCAEnabled":true,"DCAToken":"123456789","DCAUrl":"datadog-cluster-agent.bosh-deployment-name:5005",`
		expected += `"DataDogAPIKey":"*****","DataDogAdditionalEndpoints":{"https://app.datadoghq.com/api/v1/series":["*****","*****"],`
		expected += `"https://app.datadoghq.com/api/v2/series":["*****"]},`
		expected += `"DataDogAdditionalLogIntakeEndpoints":["https://http-intake.logs.us3.datadoghq.com/api/v2/logs","https://http-intake.logs.datadoghq.eu/api/v2/logs"],"DataDogLogIntakeURL":"https://http-intake.logs.datadoghq.com/api/v2/logs","DataDogTimeoutSeconds":5,`
		expected += `"DataDogURL":"https://app.datadoghq.com/api/v1/series","Deployment":"deployment-name",`
		expected += `"DeploymentFilter":"deployment-filter","DisableAccessControl":false,"EnableAdvancedTagging":true,"EnableApplicationLogs":true,"EnableMetadataAppMetricsPrefix":true,"EnableMetadataCollection":true,"EnvironmentName":"env_name",`
		expected += `"FirehoseSubscriptionID":"datadog-nozzle","FlushDurationSeconds":15,"FlushMaxBytes":57671680,`
		expected += `"GrabInterval":50,"HTTPProxyURL":"http://user:password@host.com:port",`
		expected += `"HTTPSProxyURL":"https://user:password@host.com:port","IdleTimeoutSeconds":60,"InsecureSSLSkipVerify":true,`
		expected += `"MetadataKeysBlacklistPatterns":["blacklisted1","blacklisted2"],"MetadataKeysWhitelistPatterns":["whitelisted1","whitelisted2"],`
		expected += `"MetricPrefix":"datadogclient","NoProxy":["*.aventail.com","home.com",".seanet.com"],"NumCacheWorkers":2,"NumWorkers":1,`
		expected += `"OrgDataCollectionInterval":100,"RLPGatewayURL":"https://some-url.blah",`
		expected += `"UAAURL":"https://uaa.walnut.cf-app.com","WorkerTimeoutSeconds":30}`
		conf, err := Parse("testdata/test_config.json")
		Expect(err).ToNot(HaveOccurred())
		result, err := conf.AsLogString()
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(expected))
	})
})

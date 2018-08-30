package nozzleconfig_test

import (
	"os"

	"github.com/DataDog/datadog-firehose-nozzle/nozzleconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NozzleConfig", func() {
	BeforeEach(func() {
		os.Clearenv()
	})

	It("successfully parses a valid config", func() {
		conf, err := nozzleconfig.Parse("test_config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut.cf-app.com"))
		Expect(conf.Client).To(Equal("user"))
		Expect(conf.ClientSecret).To(Equal("user_password"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq.com/api/v1/series"))
		Expect(conf.DataDogAPIKey).To(Equal("<enter api key>"))
		Expect(conf.DataDogAdditionalEndpoints).To(BeEquivalentTo(map[string][]string{
			"https://app.datadoghq.com/api/v1/series": {
				"<apikey1>",
				"<apikey2>",
			},
			"https://app.datadoghq.com/api/v2/series": {
				"<apikey3>",
			},
		}))
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
		Expect(conf.CustomTags).To(BeEquivalentTo([]string{
			"nozzle:foobar",
			"env:prod",
			"role:db",
		}))
	})

	It("successfully overwrites file config values with environmental variables", func() {
		os.Setenv("NOZZLE_UAAURL", "https://uaa.walnut-env.cf-app.com")
		os.Setenv("NOZZLE_CLIENT", "env-user")
		os.Setenv("NOZZLE_CLIENT_SECRET", "env-user-password")
		os.Setenv("NOZZLE_DATADOGURL", "https://app.datadoghq-env.com/api/v1/series")
		os.Setenv("NOZZLE_DATADOGAPIKEY", "envapi-key>")
		os.Setenv("NOZZLE_DATADOGURL_1", "https://app.datadoghq-env.com/api/v1/series")
		os.Setenv("NOZZLE_DATADOGAPIKEY_1", "envapi-key1>")
		os.Setenv("NOZZLE_DATADOGURL_2", "https://app.datadoghq-env.com/api/v1/series")
		os.Setenv("NOZZLE_DATADOGAPIKEY_2", "envapi-key2>")
		os.Setenv("NOZZLE_DATADOGURL_3", "https://app.datadoghq-env.com/api/v2/series")
		os.Setenv("NOZZLE_DATADOGAPIKEY_3", "envapi-key3>")
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
		os.Setenv("NO_PROXY", "google.com,datadoghq.com")

		conf, err := nozzleconfig.Parse("test_config.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(conf.UAAURL).To(Equal("https://uaa.walnut-env.cf-app.com"))
		Expect(conf.Client).To(Equal("env-user"))
		Expect(conf.ClientSecret).To(Equal("env-user-password"))
		Expect(conf.DataDogURL).To(Equal("https://app.datadoghq-env.com/api/v1/series"))
		Expect(conf.DataDogAPIKey).To(Equal("envapi-key>"))
		Expect(conf.DataDogAdditionalEndpoints).To(BeEquivalentTo(map[string][]string{
			"https://app.datadoghq.com/api/v1/series": {
				"envapi-key1>",
				"envapi-key2>",
			},
			"https://app.datadoghq.com/api/v2/series": {
				"envapi-key3>",
			},
		}))
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
		Expect(conf.IdleTimeoutSeconds).To(BeEquivalentTo(30))
		Expect(conf.DBPath).To(BeEquivalentTo("/var/vcap/nozzle.db"))
	})
})

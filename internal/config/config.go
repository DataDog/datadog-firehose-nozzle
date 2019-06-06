package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

const (
	defaultWorkers              int    = 4
	defaultIdleTimeoutSeconds   uint32 = 60
	defaultWorkerTimeoutSeconds uint32 = 10
)

type Config struct {
	UAAURL                     string
	Client                     string
	ClientSecret               string
	TrafficControllerURL       string
	FirehoseSubscriptionID     string
	DataDogURL                 string
	DataDogAPIKey              string
	DataDogAdditionalEndpoints map[string][]string
	HTTPProxyURL               string
	HTTPSProxyURL              string
	NoProxy                    []string
	CloudControllerEndpoint    string
	DataDogTimeoutSeconds      uint32
	FlushDurationSeconds       uint32
	FlushMaxBytes              uint32
	InsecureSSLSkipVerify      bool
	MetricPrefix               string
	Deployment                 string
	DeploymentFilter           string
	DisableAccessControl       bool
	IdleTimeoutSeconds         uint32
	AppMetrics                 bool
	NumWorkers                 int
	GrabInterval               int
	CustomTags                 []string
	DBPath                     string
	EnvironmentName            string
	WorkerTimeoutSeconds       uint32
}

func Parse(configPath string) (*Config, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	var config Config
	if err != nil {
		return nil, fmt.Errorf("Can not read config file [%s]: %s", configPath, err)
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, fmt.Errorf("Can not parse config file %s: %s", configPath, err)
	}

	overrideWithEnvVar("NOZZLE_UAAURL", &config.UAAURL)
	overrideWithEnvVar("NOZZLE_CLIENT", &config.Client)
	overrideWithEnvVar("NOZZLE_CLIENT_SECRET", &config.ClientSecret)
	overrideWithEnvVar("NOZZLE_TRAFFICCONTROLLERURL", &config.TrafficControllerURL)
	overrideWithEnvVar("NOZZLE_FIREHOSESUBSCRIPTIONID", &config.FirehoseSubscriptionID)
	overrideWithEnvVar("NOZZLE_DATADOGURL", &config.DataDogURL)
	overrideWithEnvVar("NOZZLE_DATADOGAPIKEY", &config.DataDogAPIKey)
	//NOTE: Override of DataDogAdditionalEndpoints not supported

	overrideWithEnvVar("HTTP_PROXY", &config.HTTPProxyURL)
	overrideWithEnvVar("HTTPS_PROXY", &config.HTTPSProxyURL)
	overrideWithEnvUint32("NOZZLE_DATADOGTIMEOUTSECONDS", &config.DataDogTimeoutSeconds)
	overrideWithEnvVar("NOZZLE_METRICPREFIX", &config.MetricPrefix)
	overrideWithEnvVar("NOZZLE_DEPLOYMENT", &config.Deployment)
	overrideWithEnvVar("NOZZLE_DEPLOYMENT_FILTER", &config.DeploymentFilter)

	overrideWithEnvUint32("NOZZLE_FLUSHDURATIONSECONDS", &config.FlushDurationSeconds)
	overrideWithEnvUint32("NOZZLE_FLUSHMAXBYTES", &config.FlushMaxBytes)

	overrideWithEnvBool("NOZZLE_INSECURESSLSKIPVERIFY", &config.InsecureSSLSkipVerify)
	overrideWithEnvBool("NOZZLE_DISABLEACCESSCONTROL", &config.DisableAccessControl)
	overrideWithEnvUint32("NOZZLE_IDLETIMEOUTSECONDS", &config.IdleTimeoutSeconds)
	overrideWithEnvUint32("NOZZLE_WORKERTIMEOUTSECONDS", &config.WorkerTimeoutSeconds)
	overrideWithEnvSliceStrings("NO_PROXY", &config.NoProxy)
	overrideWithEnvVar("NOZZLE_DB_PATH", &config.DBPath)
	overrideWithEnvVar("NOZZLE_ENVIRONMENT_NAME", &config.EnvironmentName)

	if config.MetricPrefix == "" {
		config.MetricPrefix = "cloudfoundry.nozzle."
	}

	if config.NumWorkers == 0 {
		config.NumWorkers = defaultWorkers
	}

	if config.IdleTimeoutSeconds == 0 {
		config.IdleTimeoutSeconds = defaultIdleTimeoutSeconds
	}

	if config.WorkerTimeoutSeconds == 0 {
		config.WorkerTimeoutSeconds = defaultWorkerTimeoutSeconds
	}

	overrideWithEnvInt("NOZZLE_NUM_WORKERS", &config.NumWorkers)

	return &config, nil
}

func overrideWithEnvVar(name string, value *string) {
	envValue := os.Getenv(name)
	if envValue != "" {
		*value = envValue
	}
}

func overrideWithEnvUint32(name string, value *uint32) {
	envValue := os.Getenv(name)
	if envValue != "" {
		tmpValue, err := strconv.Atoi(envValue)
		if err != nil {
			panic(err)
		}
		*value = uint32(tmpValue)
	}
}

func overrideWithEnvInt(name string, value *int) {
	envValue := os.Getenv(name)
	if envValue != "" {
		tmpValue, err := strconv.Atoi(envValue)
		if err != nil {
			panic(err)
		}
		*value = int(tmpValue)
	}
}

func overrideWithEnvBool(name string, value *bool) {
	envValue := os.Getenv(name)
	if envValue != "" {
		var err error
		*value, err = strconv.ParseBool(envValue)
		if err != nil {
			panic(err)
		}
	}
}

func overrideWithEnvSliceStrings(name string, value *[]string) {
	envValue := os.Getenv(name)
	*value = strings.Split(envValue, ",")
}

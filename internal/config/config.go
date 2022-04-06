package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultCloudControllerAPIBatchSize uint32 = 500
	defaultGrabInterval                int    = 10
	defaultWorkers                     int    = 4
	defaultIdleTimeoutSeconds          uint32 = 60
	defaultWorkerTimeoutSeconds        uint32 = 10
	defaultOrgDataCollectionInterval   uint32 = 600
)

var NozzleConfig Config

// Config contains all the config parameters
type Config struct {
	// NOTE: When adding new attributes that can be considered secrets,
	// make sure to mark them for omission when logging config in AsLogString
	UAAURL                        string
	Client                        string
	ClientSecret                  string
	RLPGatewayURL                 string
	FirehoseSubscriptionID        string
	DataDogURL                    string
	DataDogAPIKey                 string
	DataDogAdditionalEndpoints    map[string][]string
	HTTPProxyURL                  string
	HTTPSProxyURL                 string
	NoProxy                       []string
	CloudControllerEndpoint       string
	CloudControllerAPIBatchSize   uint32
	DataDogTimeoutSeconds         uint32
	FlushDurationSeconds          uint32
	FlushMaxBytes                 uint32
	InsecureSSLSkipVerify         bool
	MetricPrefix                  string
	Deployment                    string
	DeploymentFilter              string
	DisableAccessControl          bool
	IdleTimeoutSeconds            uint32
	AppMetrics                    bool
	NumWorkers                    int
	NumCacheWorkers               int
	GrabInterval                  int
	CustomTags                    []string
	EnvironmentName               string
	WorkerTimeoutSeconds          uint32
	OrgDataCollectionInterval     uint32
	EnableMetadataCollection      bool
	EnableAdvancedTagging         bool
	MetadataKeysWhitelistPatterns []string
	MetadataKeysBlacklistPatterns []string
	MetadataKeysWhitelist         []*regexp.Regexp `json:"-"`
	MetadataKeysBlacklist         []*regexp.Regexp `json:"-"`
	DCAEnabled                    bool
	DCAUrl                        string
	DCAToken                      string
}

// AsLogString returns a string representation of the config that is safe to log (no secrets)
func (c *Config) AsLogString() (string, error) {
	// convert the config object to map by marshalling and unmarshalling it back
	serialized, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	asMap := map[string]interface{}{}
	err = json.Unmarshal(serialized, &asMap)
	if err != nil {
		return "", err
	}

	for _, attr := range []string{"ClientSecret", "DataDogAPIKey"} {
		if _, ok := asMap[attr]; ok {
			asMap[attr] = "*****"
		}
	}
	if addKeys, ok := asMap["DataDogAdditionalEndpoints"]; ok && addKeys != nil {
		for _, v := range addKeys.(map[string]interface{}) {
			keyList := v.([]interface{})
			for i := range keyList {
				keyList[i] = "*****"
			}
		}
	}

	finalSerialized, err := json.Marshal(asMap)
	if err != nil {
		return "", err
	}
	return string(finalSerialized), nil
}

// Parse parses the config from the json configuration and environment variables
func Parse(configPath string) (Config, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	var config Config
	if err != nil {
		return Config{}, fmt.Errorf("can not read config file [%s]: %s", configPath, err)
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		return Config{}, fmt.Errorf("can not parse config file %s: %s", configPath, err)
	}

	overrideWithEnvVar("NOZZLE_UAAURL", &config.UAAURL)
	overrideWithEnvVar("NOZZLE_CLIENT", &config.Client)
	overrideWithEnvVar("NOZZLE_CLIENT_SECRET", &config.ClientSecret)
	overrideWithEnvVar("NOZZLE_RLP_GATEWAY_URL", &config.RLPGatewayURL)
	overrideWithEnvVar("NOZZLE_FIREHOSESUBSCRIPTIONID", &config.FirehoseSubscriptionID)
	overrideWithEnvVar("NOZZLE_DATADOGURL", &config.DataDogURL)
	overrideWithEnvVar("NOZZLE_DATADOGAPIKEY", &config.DataDogAPIKey)
	//NOTE: Override of DataDogAdditionalEndpoints not supported

	overrideWithEnvVar("HTTP_PROXY", &config.HTTPProxyURL)
	overrideWithEnvVar("HTTPS_PROXY", &config.HTTPSProxyURL)
	overrideWithEnvUint32("NOZZLE_CLOUD_CONTROLLER_API_BATCH_SIZE", &config.CloudControllerAPIBatchSize)
	overrideWithEnvUint32("NOZZLE_DATADOGTIMEOUTSECONDS", &config.DataDogTimeoutSeconds)
	overrideWithEnvVar("NOZZLE_METRICPREFIX", &config.MetricPrefix)
	overrideWithEnvVar("NOZZLE_DEPLOYMENT", &config.Deployment)
	overrideWithEnvVar("NOZZLE_DEPLOYMENT_FILTER", &config.DeploymentFilter)

	overrideWithEnvUint32("NOZZLE_FLUSHDURATIONSECONDS", &config.FlushDurationSeconds)
	overrideWithEnvUint32("NOZZLE_FLUSHMAXBYTES", &config.FlushMaxBytes)
	overrideWithEnvInt("NOZZLE_GRAB_INTERVAL", &config.GrabInterval)
	overrideWithEnvUint32("NOZZLE_ORG_DATA_COLLECTION_INTERVAL", &config.OrgDataCollectionInterval)

	overrideWithEnvBool("NOZZLE_INSECURESSLSKIPVERIFY", &config.InsecureSSLSkipVerify)
	overrideWithEnvBool("NOZZLE_DISABLEACCESSCONTROL", &config.DisableAccessControl)
	overrideWithEnvUint32("NOZZLE_IDLETIMEOUTSECONDS", &config.IdleTimeoutSeconds)
	overrideWithEnvUint32("NOZZLE_WORKERTIMEOUTSECONDS", &config.WorkerTimeoutSeconds)
	overrideWithEnvSliceStrings("NO_PROXY", &config.NoProxy)
	overrideWithEnvVar("NOZZLE_ENVIRONMENT_NAME", &config.EnvironmentName)
	overrideWithEnvBool("NOZZLE_ENABLE_METADATA_COLLECTION", &config.EnableMetadataCollection)

	overrideWithEnvSliceStrings("NOZZLE_METADATA_KEYS_WHITELIST", &config.MetadataKeysWhitelistPatterns)
	overrideWithEnvSliceStrings("NOZZLE_METADATA_KEYS_BLACKLIST", &config.MetadataKeysBlacklistPatterns)

	overrideWithEnvBool("NOZZLE_DCA_ENABLED", &config.DCAEnabled)
	overrideWithEnvVar("NOZZLE_DCA_URL", &config.DCAUrl)
	overrideWithEnvVar("NOZZLE_DCA_TOKEN", &config.DCAToken)
	overrideWithEnvBool("NOZZLE_DCA_ADVANCED_TAGGING", &config.EnableAdvancedTagging)

	for _, pattern := range config.MetadataKeysWhitelistPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return Config{}, fmt.Errorf("error compiling metadata key whitelist pattern %s: %v", pattern, err)
		}
		config.MetadataKeysWhitelist = append(config.MetadataKeysWhitelist, re)
	}
	for _, pattern := range config.MetadataKeysBlacklistPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return Config{}, fmt.Errorf("error compiling metadata key blacklist pattern %s: %v", pattern, err)
		}
		config.MetadataKeysBlacklist = append(config.MetadataKeysBlacklist, re)
	}

	if config.MetricPrefix == "" {
		config.MetricPrefix = "cloudfoundry.nozzle."
	}

	if config.GrabInterval == 0 {
		config.GrabInterval = defaultGrabInterval
	}

	if config.NumWorkers == 0 {
		config.NumWorkers = defaultWorkers
	}

	if config.NumCacheWorkers == 0 {
		config.NumCacheWorkers = defaultWorkers
	}

	if config.IdleTimeoutSeconds == 0 {
		config.IdleTimeoutSeconds = defaultIdleTimeoutSeconds
	}

	if config.WorkerTimeoutSeconds == 0 {
		config.WorkerTimeoutSeconds = defaultWorkerTimeoutSeconds
	}

	if config.CloudControllerAPIBatchSize == 0 {
		config.CloudControllerAPIBatchSize = defaultCloudControllerAPIBatchSize
	} else if config.CloudControllerAPIBatchSize < 100 || config.CloudControllerAPIBatchSize > 5000 {
		return Config{}, fmt.Errorf("CloudControllerAPIBatchSize must be an integer >= 100 and <= 5000")
	}

	if config.OrgDataCollectionInterval == 0 {
		config.OrgDataCollectionInterval = defaultOrgDataCollectionInterval
	}

	overrideWithEnvInt("NOZZLE_NUM_WORKERS", &config.NumWorkers)
	overrideWithEnvInt("NOZZLE_NUM_CACHE_WORKERS", &config.NumCacheWorkers)

	return config, nil
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
		*value = tmpValue
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
	if envValue != "" {
		*value = strings.Split(envValue, ",")
	}
}

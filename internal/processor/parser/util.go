package parser

import (
	"fmt"
	"strconv"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
)

func parseHost(envelope *loggregator_v2.Envelope) string {
	if index, ok := envelope.GetTags()["index"]; ok && index != "" {
		return index
	} else if origin, ok := envelope.GetTags()["origin"]; ok && origin != "" {
		return origin
	}

	return ""
}

func appendTagIfNotEmpty(tags []string, key, value string) []string {
	if value != "" {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	return tags
}

func appendMetadataTags(tags []string, metadataCollection map[string]string, keyPrefix string) []string {
	for key, value := range metadataCollection {
		if isMetadataKeyAllowed(key) {
			tags = appendTagIfNotEmpty(tags, fmt.Sprintf("%s%s", keyPrefix, key), value)
		}
	}
	return tags
}

func isMetadataKeyAllowed(key string) bool {
	// Return false if a key is blacklisted
	// Return true if a key is whitelisted or there are no blacklist nor whitelist patterns
	// Blacklist takes precedence, i.e. return false if a key matches both a whitelist and blacklist pattern

	// If there is no whitelist, assume at first the value is allowed, then refine decision based on blacklist.
	allowed := len(config.NozzleConfig.MetadataKeysWhitelist) == 0
	for _, keyRE := range config.NozzleConfig.MetadataKeysWhitelist {
		if keyRE.Match([]byte(key)) {
			allowed = true
			break
		}
	}
	for _, keyRE := range config.NozzleConfig.MetadataKeysBlacklist {
		if keyRE.Match([]byte(key)) {
			allowed = false
			break
		}
	}
	return allowed
}

func getContainerInstanceID(gauge *loggregator_v2.Gauge, instanceID string) string {
	if gauge == nil {
		return ""
	}
	if id, ok := gauge.GetMetrics()["instance_index"]; ok && id != nil {
		return strconv.Itoa(int(id.GetValue()))
	}
	return instanceID
}

package parser

import (
	"fmt"
	"strconv"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
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
		tags = appendTagIfNotEmpty(tags, fmt.Sprintf("%s.%s", keyPrefix, key), value)
	}
	return tags
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

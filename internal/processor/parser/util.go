package parser

import (
	"fmt"
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

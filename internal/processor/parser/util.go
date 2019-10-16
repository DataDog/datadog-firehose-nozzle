package parser

import (
	"fmt"
	"github.com/cloudfoundry/sonde-go/events"
)


func parseHost(envelope *events.Envelope) string {
	if envelope.GetIndex() != "" {
		return envelope.GetIndex()
	} else if envelope.GetOrigin() != "" {
		return envelope.GetOrigin()
	}

	return ""
}

func appendTagIfNotEmpty(tags []string, key, value string) []string {
	if value != "" {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	return tags
}
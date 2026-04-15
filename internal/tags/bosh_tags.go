package tags

import (
	"fmt"
	"strings"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

// BoshTagsConfig controls BOSH tag generation and hostname resolution
type BoshTagsConfig struct {
	Enabled                    bool `json:"BoshTags"`
	FriendlyHostname           bool `json:"FriendlyHostname"`
	UniqueFriendlyHostname     bool `json:"UniqueFriendlyHostname"`
	FriendlyHostnameAppendGUID bool `json:"FriendlyHostnameAppendGuid"`
	UseUUIDHostname            bool `json:"UseUuidHostname"`
}

// EnvelopeMetadata holds BOSH metadata extracted from loggregator envelopes
type EnvelopeMetadata struct {
	ID         string // VM GUID (envelope "index" tag)
	Job        string // job name like "diego-cell"
	Index      string // instance index
	AZ         string // availability zone
	Deployment string // BOSH deployment name
	Address    string // DNS address
	IP         string // IP address
}

// ExtractMetadataFromEnvelope pulls BOSH metadata from a loggregator envelope
func ExtractMetadataFromEnvelope(envelope *loggregator_v2.Envelope) EnvelopeMetadata {
	t := envelope.GetTags()
	return EnvelopeMetadata{
		ID:         t["index"],
		Job:        t["job"],
		Index:      envelope.GetInstanceId(),
		AZ:         t["az"],
		Deployment: t["deployment"],
		Address:    t["address"],
		IP:         t["ip"],
	}
}

// GenerateBoshTags creates tags matching the datadog-agent-boshrelease behavior
// https://github.com/DataDog/datadog-agent-boshrelease/blob/master/jobs/dd-agent/templates/config/datadog.yaml.erb#L133-L159
func GenerateBoshTags(config BoshTagsConfig, meta EnvelopeMetadata) []string {
	var tags []string

	if config.Enabled {
		if meta.ID != "" {
			tags = append(tags, fmt.Sprintf("bosh_id:%s", meta.ID))
		}
		if meta.Job != "" {
			tags = append(tags, fmt.Sprintf("bosh_job:%s", meta.Job))
			tags = append(tags, fmt.Sprintf("bosh_name:%s", meta.Job))
		}
		if meta.Index != "" {
			tags = append(tags, fmt.Sprintf("bosh_index:%s", meta.Index))
		}
		if meta.AZ != "" {
			tags = append(tags, fmt.Sprintf("bosh_az:%s", meta.AZ))
		}
		if meta.Deployment != "" {
			tags = append(tags, fmt.Sprintf("bosh_deployment:%s", meta.Deployment))
		}
		if meta.Address != "" {
			tags = append(tags, fmt.Sprintf("bosh_address:%s", meta.Address))
		}
		if meta.IP != "" {
			tags = append(tags, fmt.Sprintf("bosh_ip:%s", meta.IP))
		}
	}

	// These are always added (not optional per the Ruby code)
	if meta.Deployment != "" {
		if !config.Enabled {
			tags = append(tags, fmt.Sprintf("bosh_deployment:%s", meta.Deployment))
		}
		tags = append(tags, fmt.Sprintf("deployment:%s", meta.Deployment))
	}

	return tags
}

// GenerateHostname creates hostname matching the datadog-agent-boshrelease behavior
// https://github.com/DataDog/datadog-agent-boshrelease/blob/master/jobs/dd-agent/templates/config/datadog.yaml.erb#L100-L124
func GenerateHostname(config BoshTagsConfig, meta EnvelopeMetadata) string {
	var hostname string

	if config.UseUUIDHostname && meta.ID != "" {
		hostname = meta.ID
	} else if config.FriendlyHostname {
		hostname = fmt.Sprintf("%s-%s", meta.Job, meta.Index)

		if config.UniqueFriendlyHostname {
			hostname = fmt.Sprintf("%s-%s", hostname, meta.Deployment)
		}
		if config.FriendlyHostnameAppendGUID && meta.ID != "" {
			hostname = fmt.Sprintf("%s-%s", hostname, meta.ID)
		}
	} else {
		// Fallback to job-index
		hostname = fmt.Sprintf("%s-%s", meta.Job, meta.Index)
	}

	// Replace underscores with hyphens (matching Ruby: hostname.tr('_', '-'))
	hostname = strings.ReplaceAll(hostname, "_", "-")

	return hostname
}

package metricProcessor

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/sonde-go/events"
)

func (p *Processor) ParseInfraMetric(envelope *events.Envelope) ([]metrics.MetricPackage, error) {
	metricsPackages := []metrics.MetricPackage{}

	if envelope.GetEventType() != events.Envelope_ValueMetric && envelope.GetEventType() != events.Envelope_CounterEvent {
		return metricsPackages, fmt.Errorf("not an infra metric")
	}

	host := parseHost(envelope)
	tags := parseTags(envelope, p.environment, p.deploymentUUIDRegex, p.jobPartitionUUIDRegex)
	tags = append(tags, p.customTags...)
	name := getName(envelope)

	key := metrics.MetricKey{
		EventType: envelope.GetEventType(),
		Name:      name,
		TagsHash:  utils.HashTags(tags),
	}

	mVal := metrics.MetricValue{}
	value := getValue(envelope)

	mVal.Host = host
	mVal.Tags = tags
	mVal.Points = append(mVal.Points, metrics.Point{
		Timestamp: envelope.GetTimestamp() / int64(time.Second),
		Value:     value,
	})

	metricsPackages = append(metricsPackages, metrics.MetricPackage{
		MetricKey:   &key,
		MetricValue: &mVal,
	})

	if strings.HasPrefix(name, "bosh-hm-forwarder") {
		newName := strings.Replace(name, "bosh-hm-forwarder", "bosh.healthmonitor", 1)
		newKey := metrics.MetricKey{
			EventType: envelope.GetEventType(),
			Name:      newName,
			TagsHash:  utils.HashTags(tags),
		}

		metricsPackages = append(metricsPackages, metrics.MetricPackage{
			MetricKey:   &newKey,
			MetricValue: &mVal,
		})
	}

	keyLegacyName := metrics.MetricKey{
		EventType: envelope.GetEventType(),
		Name:      envelope.GetOrigin() + "." + key.Name,
		TagsHash:  utils.HashTags(tags),
	}

	metricsPackages = append(metricsPackages, metrics.MetricPackage{
		MetricKey:   &keyLegacyName,
		MetricValue: &mVal,
	})

	return metricsPackages, nil
}

func getName(envelope *events.Envelope) string {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetValueMetric().GetName()
	case events.Envelope_CounterEvent:
		return envelope.GetCounterEvent().GetName()
	default:
		panic("Unknown event type")
	}
}

func getValue(envelope *events.Envelope) float64 {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetValueMetric().GetValue()
	case events.Envelope_CounterEvent:
		return float64(envelope.GetCounterEvent().GetTotal())
	default:
		panic("Unknown event type")
	}
}

func parseTags(envelope *events.Envelope, environment string, deploymentUUIDPattern *regexp.Regexp, jobPartitionUUIDPattern *regexp.Regexp) []string {
	tags := appendTagIfNotEmpty(nil, "deployment", envelope.GetDeployment())
	tags = appendTagIfNotEmpty(tags, "job", envelope.GetJob())
	tags = appendTagIfNotEmpty(tags, "index", envelope.GetIndex())
	tags = appendTagIfNotEmpty(tags, "ip", envelope.GetIp())
	tags = appendTagIfNotEmpty(tags, "origin", envelope.GetOrigin())
	tags = appendTagIfNotEmpty(tags, "name", envelope.GetOrigin())
	for tname, tvalue := range envelope.GetTags() {
		tags = appendTagIfNotEmpty(tags, tname, tvalue)
	}

	// Add an environment tag and another deployment tag with the uuid part replaced with environment name
	tags = appendTagIfNotEmpty(tags, "env", environment)
	newDeploymentTag := deploymentUUIDPattern.ReplaceAllString(envelope.GetDeployment(), "")
	if environment != "" {
		tags = appendTagIfNotEmpty(tags, "deployment", fmt.Sprintf("%s_%s", newDeploymentTag, environment))
	}
	// Do not duplicate tag
	if newDeploymentTag != envelope.GetDeployment() {
		tags = appendTagIfNotEmpty(tags, "deployment", newDeploymentTag)
	}

	// Add a new job tag with the partition uuid part replaced with its index an one with only the job name
	newJobTag := jobPartitionUUIDPattern.ReplaceAllString(envelope.GetJob(), "")
	// Do not duplicate tag
	if newJobTag != envelope.GetJob() {
		tags = appendTagIfNotEmpty(tags, "job", newJobTag)
	}

	return tags
}

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

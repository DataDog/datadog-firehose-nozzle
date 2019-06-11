package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"
	"github.com/cloudfoundry/sonde-go/events"
)

type InfraParser struct {
	Environment           string
	DeploymentUUIDRegex   *regexp.Regexp
	JobPartitionUUIDRegex *regexp.Regexp
	CustomTags            []string
}

func NewInfraParser(
	environment string,
	deploymentUUIDRegex *regexp.Regexp,
	jobPartitionUUIDRegex *regexp.Regexp,
	customTags []string) (*InfraParser, error) {
	return &InfraParser{
		Environment:           environment,
		DeploymentUUIDRegex:   deploymentUUIDRegex,
		JobPartitionUUIDRegex: jobPartitionUUIDRegex,
		CustomTags:            customTags,
	}, nil
}

func (p InfraParser) Parse(envelope *events.Envelope) ([]metric.MetricPackage, error) {
	metrics := []metric.MetricPackage{}

	eventType := envelope.GetEventType()
	if eventType != events.Envelope_ValueMetric && eventType != events.Envelope_CounterEvent {
		return metrics, fmt.Errorf("not an infra metric")
	}

	host := parseHost(envelope)
	name := getName(envelope)
	tags := parseTags(envelope, p.Environment, p.DeploymentUUIDRegex, p.JobPartitionUUIDRegex)
	tags = append(tags, p.CustomTags...)
	tagsHash := util.HashTags(tags)

	// create metricValues
	metricValues := metric.MetricValue{}
	metricValues.Host = host
	metricValues.Tags = tags
	value := getValue(envelope)
	// NOTE: Value only has one point!!!!!!!!
	metricValues.Points = append(metricValues.Points, metric.Point{
		Timestamp: envelope.GetTimestamp() / int64(time.Second),
		Value:     value,
	})

	var names []string
	// Basic metric name
	names = append(names, name)
	// Legacy metric name
	names = append(names, envelope.GetOrigin()+"."+name)
	// BOSH alias metric name
	if strings.HasPrefix(name, "bosh-hm-forwarder") {
		names = append(names, strings.Replace(name, "bosh-hm-forwarder", "bosh.healthmonitor", 1))
	}

	// Create metric for each names with same values
	for i := 0; i < len(names); i++ {
		metrics = append(metrics, metric.MetricPackage{
			MetricKey: &metric.MetricKey{
				EventType: eventType,
				Name:      names[i],
				TagsHash:  tagsHash,
			},
			MetricValue: &metricValues,
		})
	}

	return metrics, nil
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

func parseTags(
	envelope *events.Envelope,
	environment string,
	deploymentUUIDRegex *regexp.Regexp,
	jobPartitionUUIDRegex *regexp.Regexp) []string {
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
	newDeploymentTag := deploymentUUIDRegex.ReplaceAllString(envelope.GetDeployment(), "")
	if environment != "" {
		tags = appendTagIfNotEmpty(tags, "deployment", fmt.Sprintf("%s_%s", newDeploymentTag, environment))
	}
	// Do not duplicate tag
	if newDeploymentTag != envelope.GetDeployment() {
		tags = appendTagIfNotEmpty(tags, "deployment", newDeploymentTag)
	}

	// Add a new job tag with the partition uuid part replaced with its index an one with only the job name
	newJobTag := jobPartitionUUIDRegex.ReplaceAllString(envelope.GetJob(), "")
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

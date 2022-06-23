package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/logs"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
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

func (p InfraParser) Parse(envelope *loggregator_v2.Envelope) ([]metric.MetricPackage, error) {
	metrics := []metric.MetricPackage{}

	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Counter:
		break
	case *loggregator_v2.Envelope_Gauge:
		if !util.IsContainerMetric(envelope) {
			break
		}
		// if it's container metric, return error (we don't handle container metrics here)
		// NOTE: `fallthrough` is not allowed in type switch
		return metrics, fmt.Errorf("not an infra metric")
	default:
		return metrics, fmt.Errorf("not an infra metric")
	}

	host := parseHost(envelope)
	tags := parseTags(envelope, p.Environment, p.DeploymentUUIDRegex, p.JobPartitionUUIDRegex)
	tags = append(tags, p.CustomTags...)
	tagsHash := util.HashTags(tags)

	for name, value := range getValues(envelope) {
		// create metricValues
		metricValues := metric.MetricValue{}
		metricValues.Host = host
		metricValues.Tags = tags
		metricValues.Points = append(metricValues.Points, metric.Point{
			Timestamp: envelope.GetTimestamp() / int64(time.Second),
			Value:     value,
		})

		var names []string
		// Basic metric name
		names = append(names, name)
		// Legacy metric name
		if origin, ok := envelope.GetTags()["origin"]; ok {
			names = append(names, origin+"."+name)
		}
		// BOSH alias metric name
		if strings.HasPrefix(name, "bosh-hm-forwarder") {
			names = append(names, strings.Replace(name, "bosh-hm-forwarder", "bosh.healthmonitor", 1))
		}

		// Create metric for each names with same values
		for i := 0; i < len(names); i++ {
			metrics = append(metrics, metric.MetricPackage{
				MetricKey: &metric.MetricKey{
					Name:     names[i],
					TagsHash: tagsHash,
				},
				MetricValue: &metricValues,
			})
		}
	}

	return metrics, nil
}

func (p InfraParser) ParseLog(envelope *loggregator_v2.Envelope) (logs.LogMessage, error) {
	logValue := logs.LogMessage{}

	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Log:
		break
	default:
		return logValue, fmt.Errorf("not a log envelope")
	}

	host := parseHost(envelope)
	tags := parseTags(envelope, p.Environment, p.DeploymentUUIDRegex, p.JobPartitionUUIDRegex)
	tags = append(tags, p.CustomTags...)

	logValue.Hostname = host
	logValue.Tags = strings.Join(tags, ",")
	logValue.Message = string(envelope.GetLog().Payload)
	logValue.Source = envelope.SourceId
	logValue.Service = envelope.GetTags()["process_id"]

	return logValue, nil
}

func getValues(envelope *loggregator_v2.Envelope) map[string]float64 {
	values := map[string]float64{}
	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Gauge:
		for k, v := range envelope.GetGauge().GetMetrics() {
			values[k] = v.Value
		}
	case *loggregator_v2.Envelope_Counter:
		values[envelope.GetCounter().GetName()] = float64(envelope.GetCounter().GetTotal())
	default:
		panic("Unknown event type")
	}
	return values
}

func parseTags(
	envelope *loggregator_v2.Envelope,
	environment string,
	deploymentUUIDRegex *regexp.Regexp,
	jobPartitionUUIDRegex *regexp.Regexp) []string {

	var tags []string
	for tname, tvalue := range envelope.GetTags() {
		tags = appendTagIfNotEmpty(tags, tname, tvalue)
	}
	if origin, ok := envelope.GetTags()["origin"]; ok {
		tags = appendTagIfNotEmpty(tags, "name", origin)
	}
	tags = appendTagIfNotEmpty(tags, "instance_id", envelope.GetInstanceId())
	tags = appendTagIfNotEmpty(tags, "source_id", envelope.GetSourceId())

	// Add an environment tag and another deployment tag with the uuid part replaced with environment name
	tags = appendTagIfNotEmpty(tags, "env", environment)
	if deployment, ok := envelope.GetTags()["deployment"]; ok {
		newDeploymentTag := deploymentUUIDRegex.ReplaceAllString(deployment, "")
		if environment != "" {
			tags = appendTagIfNotEmpty(tags, "deployment", fmt.Sprintf("%s_%s", newDeploymentTag, environment))
		}
		// Do not duplicate tag
		if newDeploymentTag != deployment {
			tags = appendTagIfNotEmpty(tags, "deployment", newDeploymentTag)
		}
	}

	// Add a new job tag with the partition uuid part replaced with its index an one with only the job name
	if job, ok := envelope.GetTags()["job"]; ok {
		newJobTag := jobPartitionUUIDRegex.ReplaceAllString(job, "")
		// Do not duplicate tag
		if newJobTag != job {
			tags = appendTagIfNotEmpty(tags, "job", newJobTag)
		}
	}

	return tags
}

package metricProcessor

import (
	"regexp"

	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/cloudfoundry/sonde-go/events"
)

const (
	deploymentUUIDPattern   = "-([0-9a-f]{20})"
	jobPartitionUUIDPattern = "-partition-([0-9a-f]{20})"
)

type Processor struct {
	processedMetrics      chan<- []metrics.MetricPackage
	appMetrics            *appmetrics.AppMetrics
	customTags            []string
	environment           string
	deploymentUUIDRegex   *regexp.Regexp
	jobPartitionUUIDRegex *regexp.Regexp
}

func New(pm chan<- []metrics.MetricPackage, customTags []string, environment string) *Processor {
	return &Processor{
		processedMetrics:      pm,
		customTags:            customTags,
		environment:           environment,
		deploymentUUIDRegex:   regexp.MustCompile(deploymentUUIDPattern),
		jobPartitionUUIDRegex: regexp.MustCompile(jobPartitionUUIDPattern),
	}
}

func (p *Processor) SetAppMetrics(appMetrics *appmetrics.AppMetrics) {
	p.appMetrics = appMetrics
}

func (p *Processor) ProcessMetric(envelope *events.Envelope) {
	var err error
	var metricsPackages []metrics.MetricPackage

	metricsPackages, err = p.ParseInfraMetric(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
		// it can only be one or the other
		return
	}

	metricsPackages, err = p.ParseAppMetric(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
	}
}

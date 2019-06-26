package processor

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/processor/parser"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
)

const (
	deploymentUUIDPattern   = "-([0-9a-f]{20})"
	jobPartitionUUIDPattern = "-partition-([0-9a-f]{20})"
)

type Processor struct {
	processedMetrics      chan<- []metric.MetricPackage
	appMetrics            parser.Parser
	customTags            []string
	environment           string
	deploymentUUIDRegex   *regexp.Regexp
	jobPartitionUUIDRegex *regexp.Regexp
}

func NewProcessor(
	pm chan<- []metric.MetricPackage,
	customTags []string,
	environment string,
	parseAppMetricsEnable bool,
	cfClient *cfclient.Client,
	grabInterval int,
	log *gosteno.Logger,
) (*Processor, bool) {

	processor := &Processor{
		processedMetrics:      pm,
		customTags:            customTags,
		environment:           environment,
		deploymentUUIDRegex:   regexp.MustCompile(deploymentUUIDPattern),
		jobPartitionUUIDRegex: regexp.MustCompile(jobPartitionUUIDPattern),
	}

	if parseAppMetricsEnable {
		appMetrics, err := parser.NewAppParser(
			cfClient,
			grabInterval,
			log,
			customTags,
			environment,
		)
		if err != nil {
			parseAppMetricsEnable = false
			log.Warnf("error setting up appMetrics, continuing without application metrics: %v", err)
		} else {
			log.Debug("setting up app metrics")
			processor.appMetrics = appMetrics
		}
	}

	return processor, parseAppMetricsEnable
}

func (p *Processor) ProcessMetric(envelope *events.Envelope) {
	var err error
	var metricsPackages []metric.MetricPackage

	// Parse infrastructure type of envelopes
	infraParser, err := parser.NewInfraParser(
		p.environment,
		p.deploymentUUIDRegex,
		p.jobPartitionUUIDRegex,
		p.customTags,
	)
	metricsPackages, err = infraParser.Parse(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
		// it can only be one or the other
		return
	}

	// Parse application type of envelopes
	metricsPackages, err = p.parseAppMetric(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
	}
}

// StopAppMetrics stops the goroutine refreshing the apps cache
func (p *Processor) StopAppMetrics() {
	if p.appMetrics == nil {
		return
	}

	p.appMetrics.Stop()
}

func (p *Processor) parseAppMetric(envelope *events.Envelope) ([]metric.MetricPackage, error) {
	var metricsPackages []metric.MetricPackage
	var err error

	if p.appMetrics == nil {
		return metricsPackages, fmt.Errorf("app metrics are not configured")
	}

	if envelope.GetEventType() != events.Envelope_ContainerMetric {
		return metricsPackages, fmt.Errorf("not an app metric")
	}

	metricsPackages, err = p.appMetrics.Parse(envelope)

	return metricsPackages, err
}

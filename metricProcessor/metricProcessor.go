package metricProcessor

import (
	"github.com/DataDog/datadog-firehose-nozzle/appmetrics"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/cloudfoundry/sonde-go/events"
)

type Processor struct {
	processedMetrics chan<- []metrics.MetricPackage
	appMetrics       *appmetrics.AppMetrics
}

func New(pm chan<- []metrics.MetricPackage) *Processor {
	return &Processor{processedMetrics: pm}
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

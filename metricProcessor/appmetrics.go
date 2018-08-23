package metricProcessor

import (
	"fmt"

	"github.com/cloudfoundry/sonde-go/events"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
)

func (p *Processor) ParseAppMetric(envelope *events.Envelope) ([]metrics.MetricPackage, error) {
	var metricsPackages []metrics.MetricPackage
	var err error

	if p.appMetrics == nil {
		return metricsPackages, fmt.Errorf("app metrics are not configured")
	}

	if envelope.GetEventType() != events.Envelope_ContainerMetric {
		return metricsPackages, fmt.Errorf("not an app metric")
	}

	metricsPackages, err = p.appMetrics.ParseAppMetric(envelope)

	return metricsPackages, err
}

package parser

import (
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/sonde-go/events"
)

type Parser interface {
	Parse(envelope *events.Envelope) ([]metric.MetricPackage, error)
}

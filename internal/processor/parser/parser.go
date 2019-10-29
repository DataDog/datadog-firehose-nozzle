package parser

import (
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

type Parser interface {
	Parse(envelope *loggregator_v2.Envelope) ([]metric.MetricPackage, error)
}

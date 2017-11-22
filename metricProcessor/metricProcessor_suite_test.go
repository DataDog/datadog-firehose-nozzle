package metricProcessor_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMetricProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metric Processor Suite")
}

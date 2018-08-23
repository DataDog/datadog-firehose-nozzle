package metricProcessor_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMetricProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metric Processor Suite")
}

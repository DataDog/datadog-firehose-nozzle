package processor

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metric Processor Suite")
}

package nozzle

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatadogfirehosenozzle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DatadogFirehoseNozzle Suite")
}

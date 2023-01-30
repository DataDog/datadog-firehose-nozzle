package datadog

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatadogclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DatadogClient Suite")
}

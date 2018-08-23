package datadogclient_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDatadogclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DatadogClient Suite")
}

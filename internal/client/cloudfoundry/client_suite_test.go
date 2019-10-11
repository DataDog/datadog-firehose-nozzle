package cloudfoundry

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDatadogclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cloud Foundry Client Suite")
}

package nozzleconfig_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNozzleConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Nozzle Config Suite")
}

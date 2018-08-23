package nozzleconfig_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestNozzleConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Nozzle Config Suite")
}

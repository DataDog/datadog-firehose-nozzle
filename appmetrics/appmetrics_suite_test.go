package appmetrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAppmetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Metrics Suite")
}

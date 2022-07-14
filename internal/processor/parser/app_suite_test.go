package parser

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestAppmetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Metrics Suite")
}

package uaatokenfetcher_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUaaTokenFetcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UaaTokenFetcher Suite")
}

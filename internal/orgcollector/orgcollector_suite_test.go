package orgcollector

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestOrgCollector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OrgCollector Suite")
}

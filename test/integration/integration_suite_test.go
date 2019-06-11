package integration

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"

	"github.com/onsi/gomega/gexec"
	"os"
)

func TestDatadogFirehoseNozzle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var pathToNozzleExecutable string

var _ = BeforeSuite(func() {
	var err error
	pathToNozzleExecutable, err = gexec.Build("github.com/DataDog/datadog-firehose-nozzle")
	fmt.Println(pathToNozzleExecutable)
	fmt.Println("file exists", Exists(pathToNozzleExecutable))
	Expect(err).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

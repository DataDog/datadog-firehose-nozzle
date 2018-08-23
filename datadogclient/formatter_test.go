package datadogclient_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
)

var _ = Describe("Formatter", func() {
	var (
		formatter datadogclient.Formatter
	)

	BeforeEach(func() {
		formatter = datadogclient.Formatter{}
	})

	It("does not return empty data", func() {
		result := formatter.Format("some-prefix", 1024, nil)
		Expect(result).To(HaveLen(0))
	})

	It("does not 'delete' points when trying to split", func() {
		m := make(map[metrics.MetricKey]metrics.MetricValue)
		m[metrics.MetricKey{Name: "a"}] = metrics.MetricValue{
			Points: []metrics.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("some-prefix", 1, m)

		Expect(result).To(HaveLen(1))
	})
})

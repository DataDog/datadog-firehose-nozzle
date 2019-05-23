package datadogclient_test

import (
	"github.com/DataDog/datadog-firehose-nozzle/datadogclient"
	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/testhelpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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

	It("compresses series with zlib", func() {
		m := make(map[metrics.MetricKey]metrics.MetricValue)
		m[metrics.MetricKey{Name: "bar"}] = metrics.MetricValue{
			Points: []metrics.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("foo", 1024, m)
		Expect(string(testhelpers.Decompress(result[0]))).To(Equal(`{"series":[{"metric":"foobar","points":[[0,9.000000]],"type":"gauge"}]}`))
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

	It("does not prepend prefix to `bosh.healthmonitor`", func() {
		m := make(map[metrics.MetricKey]metrics.MetricValue)
		m[metrics.MetricKey{Name: "bosh.healthmonitor.foo"}] = metrics.MetricValue{
			Points: []metrics.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("some-prefix", 1024, m)

		Expect(string(testhelpers.Decompress(result[0]))).To(ContainSubstring(`"metric":"bosh.healthmonitor.foo"`))
	})
})

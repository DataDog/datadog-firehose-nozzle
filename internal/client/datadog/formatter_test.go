package datadog

import (
	"math"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/test/helper"
	"github.com/cloudfoundry/gosteno"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Formatter", func() {
	var (
		formatter  Formatter
		metricsMap metric.MetricsMap
	)

	BeforeEach(func() {
		formatter = Formatter{gosteno.NewLogger("test")}
		metricsMap = make(metric.MetricsMap)
	})

	It("does not return empty data", func() {
		result := formatter.Format("some-prefix", 1024, nil)
		Expect(result).To(HaveLen(0))
	})

	It("compresses series with zlib", func() {
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "bar"}] = metric.MetricValue{
			Points: []metric.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("foo", 1024, m)
		Expect(string(helper.Decompress(result[0]))).To(Equal(`{"series":[{"metric":"foobar","points":[[0,9.000000]],"type":"gauge"}]}`))
	})

	It("drops metrics that are larger than maxPostBytes", func() {
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "a"}] = metric.MetricValue{
			Points: []metric.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("some-prefix", 1, m)

		Expect(result).To(HaveLen(0))
	})

	It("does not prepend prefix to `bosh.healthmonitor`", func() {
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "bosh.healthmonitor.foo"}] = metric.MetricValue{
			Points: []metric.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("some-prefix", 1024, m)

		Expect(string(helper.Decompress(result[0]))).To(ContainSubstring(`"metric":"bosh.healthmonitor.foo"`))
	})

	It("drops metrics that have a NAN value", func() {
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "bosh.healthmonitor.foo"}] = metric.MetricValue{
			Points: []metric.Point{{
				Value: 9,
			}, {
				Value: math.Log(-1.0), //creates a NAN
			}, {
				Value: math.Log(-2.0), //creates a NAN
			}, {
				Value: 1.0,
			}},
		}
		result := formatter.Format("some-prefix", 1024, m)
		Expect(string(helper.Decompress(result[0]))).To(ContainSubstring(`"metric":"bosh.healthmonitor.foo"`))
		Expect(string(helper.Decompress(result[0]))).To(ContainSubstring(`"points":[[0,9.000000],[0,1.000000]]`))
	})

	It("properly splits metrics into two maps", func() {
		// now test a scenario where we're actually splitting metrics
		for i := 0; i < 1000; i++ {
			k, v := makeFakeMetric("metricName", 1000, uint64(i), defaultTags)
			metricsMap.Add(k, v)
		}

		a, b := splitMetrics(metricsMap)

		Expect(len(a) + len(b)).To(Equal(len(metricsMap)))
		Expect(len(a)).To(Equal(len(metricsMap) - len(metricsMap)/2))
		Expect(len(b)).To(Equal(len(metricsMap) / 2))
	})
})

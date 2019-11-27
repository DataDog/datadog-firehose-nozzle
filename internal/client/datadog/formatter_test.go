package datadog

import (
	"encoding/json"
	"math"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/test/helper"
	"github.com/cloudfoundry/gosteno"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Formatter", func() {
	var (
		formatter Formatter
	)

	BeforeEach(func() {
		formatter = Formatter{gosteno.NewLogger("test")}
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

	It("does not 'delete' points when trying to split", func() {
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "a"}] = metric.MetricValue{
			Points: []metric.Point{{
				Value: 9,
			}},
		}
		result := formatter.Format("some-prefix", 1, m)

		Expect(result).To(HaveLen(1))
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

	It("properly assigns all values to split results when splitting", func() {
		// first test a scenario where we're not splitting as there's just one point
		m := make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "a"}] = metric.MetricValue{
			Points: []metric.Point{{Value: 9}},
			Tags:   []string{"some:tag", "other:tag"},
			Host:   "some.host",
		}
		result := formatter.Format("some-prefix.", 1, m)

		Expect(result).To(HaveLen(1))

		decompressed := helper.Decompress(result[0])
		payload := Payload{}
		err := json.Unmarshal(decompressed, &payload)
		Expect(err).To(BeNil())
		Expect(payload.Series).To(HaveLen(1))
		s := payload.Series[0]
		Expect(s.Metric).To(Equal("some-prefix.a"))
		Expect(s.Points).To(Equal([]metric.Point{{Value: 9}}))
		Expect(s.Type).To(Equal("gauge"))
		Expect(s.Host).To(Equal("some.host"))
		Expect(s.Tags).To(Equal([]string{"some:tag", "other:tag"}))

		// now test a scenario where we're actually splitting points
		m = make(map[metric.MetricKey]metric.MetricValue)
		m[metric.MetricKey{Name: "a"}] = metric.MetricValue{
			Points: []metric.Point{{Value: 9}, {Value: 10}},
			Tags:   []string{"some:tag", "other:tag"},
			Host:   "some.host",
		}
		result = formatter.Format("some-prefix.", 1, m)

		Expect(result).To(HaveLen(2))

		decompressed1 := helper.Decompress(result[0])
		payload1 := Payload{}
		err1 := json.Unmarshal(decompressed1, &payload1)
		Expect(err1).To(BeNil())
		Expect(payload1.Series).To(HaveLen(1))
		s1 := payload1.Series[0]
		Expect(s1.Metric).To(Equal("some-prefix.a"))
		Expect(s1.Points).To(Equal([]metric.Point{{Value: 9}}))
		Expect(s1.Type).To(Equal("gauge"))
		Expect(s1.Host).To(Equal("some.host"))
		Expect(s1.Tags).To(Equal([]string{"some:tag", "other:tag"}))

		decompressed2 := helper.Decompress(result[1])
		payload2 := Payload{}
		err2 := json.Unmarshal(decompressed2, &payload2)
		Expect(err2).To(BeNil())
		Expect(payload2.Series).To(HaveLen(1))
		s2 := payload2.Series[0]
		Expect(s2.Metric).To(Equal("some-prefix.a"))
		Expect(s2.Points).To(Equal([]metric.Point{{Value: 10}}))
		Expect(s2.Type).To(Equal("gauge"))
		Expect(s2.Host).To(Equal("some.host"))
		Expect(s2.Tags).To(Equal([]string{"some:tag", "other:tag"}))
	})
})

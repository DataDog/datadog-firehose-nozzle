package processor

import (
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	mchan chan []metric.MetricPackage
	p     *Processor
)

var _ = Describe("MetricProcessor", func() {
	BeforeEach(func() {
		mchan = make(chan []metric.MetricPackage, 1500)
		p, _ = NewProcessor(mchan, []string{}, "", false,
			nil, 0, nil)
	})

	It("processes value & counter metrics", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("valueName"),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(2000000000),
			EventType: events.Envelope_CounterEvent.Enum(),
			CounterEvent: &events.CounterEvent{
				Name:  proto.String("counterName"),
				Delta: proto.Uint64(6),
				Total: proto.Uint64(11),
			},
		})

		var metricPkg1 []metric.MetricPackage
		Eventually(mchan).Should(Receive(&metricPkg1))

		var metricPkg2 []metric.MetricPackage
		Eventually(mchan).Should(Receive(&metricPkg2))

		metricPkgs := append(metricPkg1, metricPkg2...)

		Expect(metricPkgs).To(HaveLen(4))
		for _, m := range metricPkgs {
			if m.MetricKey.Name == "valueName" || m.MetricKey.Name == "origin.valueName" {
				Expect(m.MetricValue.Points).To(Equal([]metric.Point{{Timestamp: 1, Value: 5.0}}))
			} else if m.MetricKey.Name == "counterName" || m.MetricKey.Name == "origin.counterName" {
				Expect(m.MetricValue.Points).To(Equal([]metric.Point{{Timestamp: 2, Value: 11.0}}))
			} else {
				panic("unknown metric in package: " + m.MetricKey.Name)
			}
		}
	})

	It("generates metrics twice: once with origin in name, once without", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("fooMetric"),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		var metricPkg []metric.MetricPackage
		Eventually(mchan).Should(Receive(&metricPkg))

		Expect(metricPkg).To(HaveLen(2))

		legacyFound := false
		newFound := false
		for _, m := range metricPkg {
			if m.MetricKey.Name == "origin.fooMetric" {
				legacyFound = true
			} else if m.MetricKey.Name == "fooMetric" {
				newFound = true
			}
		}
		Expect(legacyFound).To(BeTrue())
		Expect(newFound).To(BeTrue())
	})

	It("adds a new alias for `bosh-hm-forwarder` metrics", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("bosh-hm-forwarder.foo"),
				Value: proto.Float64(5),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		var metricPkg []metric.MetricPackage
		Eventually(mchan).Should(Receive(&metricPkg))

		Expect(metricPkg).To(HaveLen(3))

		legacyFound := false
		newFound := false
		boshAliasFound := false
		for _, m := range metricPkg {
			if m.MetricKey.Name == "origin.bosh-hm-forwarder.foo" {
				legacyFound = true
			} else if m.MetricKey.Name == "bosh-hm-forwarder.foo" {
				newFound = true
			} else if m.MetricKey.Name == "bosh.healthmonitor.foo" {
				boshAliasFound = true
			}
		}
		Expect(legacyFound).To(BeTrue())
		Expect(newFound).To(BeTrue())
		Expect(boshAliasFound).To(BeTrue())
	})

	It("ignores messages that aren't value metrics or counter events", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_LogMessage.Enum(),
			LogMessage: &events.LogMessage{
				Message:     []byte("log message"),
				MessageType: events.LogMessage_OUT.Enum(),
				Timestamp:   proto.Int64(1000000000),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ContainerMetric.Enum(),
			ContainerMetric: &events.ContainerMetric{
				ApplicationId: proto.String("app-id"),
				InstanceIndex: proto.Int32(4),
				CpuPercentage: proto.Float64(20.0),
				MemoryBytes:   proto.Uint64(19939949),
				DiskBytes:     proto.Uint64(29488929),
			},
			Deployment: proto.String("deployment-name"),
			Job:        proto.String("doppler"),
		})

		Consistently(mchan).ShouldNot(Receive())
	})

	It("adds tags", func() {
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name-aaaaaaaaaaaaaaaaaaaa"),
			Job:        proto.String("doppler-partition-aaaaaaaaaaaaaaaaaaaa"),
			Ip:         proto.String("10.0.1.2"),

			// additional tags
			Tags: map[string]string{
				"protocol":   "http",
				"request_id": "a1f5-deadbeef",
			},
		})

		var metricPkg []metric.MetricPackage
		Eventually(mchan).Should(Receive(&metricPkg))

		Expect(metricPkg).To(HaveLen(2))
		for _, m := range metricPkg {
			Expect(m.MetricValue.Tags).To(Equal([]string{
				"deployment:deployment-name",
				"deployment:deployment-name-aaaaaaaaaaaaaaaaaaaa",
				"ip:10.0.1.2",
				"job:doppler",
				"job:doppler-partition-aaaaaaaaaaaaaaaaaaaa",
				"name:test-origin",
				"origin:test-origin",
				"protocol:http",
				"request_id:a1f5-deadbeef",
			}))
		}

		// Check it does the correct dogate tag replacements when env_name and index are set
		p.environment = "env_name"
		p.ProcessMetric(&events.Envelope{
			Origin:    proto.String("test-origin"),
			Timestamp: proto.Int64(1000000000),
			EventType: events.Envelope_ValueMetric.Enum(),

			// fields that gets sent as tags
			Deployment: proto.String("deployment-name-aaaaaaaaaaaaaaaaaaaa"),
			Job:        proto.String("doppler-partition-aaaaaaaaaaaaaaaaaaaa"),
			Index:      proto.String("1"),
			Ip:         proto.String("10.0.1.2"),

			// additional tags
			Tags: map[string]string{
				"protocol":   "http",
				"request_id": "a1f5-deadbeef",
			},
		})

		Eventually(mchan).Should(Receive(&metricPkg))

		Expect(metricPkg).To(HaveLen(2))
		for _, m := range metricPkg {
			Expect(m.MetricValue.Tags).To(Equal([]string{
				"deployment:deployment-name",
				"deployment:deployment-name-aaaaaaaaaaaaaaaaaaaa",
				"deployment:deployment-name_env_name",
				"env:env_name",
				"index:1",
				"ip:10.0.1.2",
				"job:doppler",
				"job:doppler-partition-aaaaaaaaaaaaaaaaaaaa",
				"name:test-origin",
				"origin:test-origin",
				"protocol:http",
				"request_id:a1f5-deadbeef",
			}))
		}
	})

	Context("custom tags", func() {
		BeforeEach(func() {
			mchan = make(chan []metric.MetricPackage, 1500)
			p, _ = NewProcessor(mchan, []string{"environment:foo", "foundry:bar"}, "", false,
				nil, 0, nil)
		})

		It("adds custom tags to infra metrics", func() {
			p.ProcessMetric(&events.Envelope{
				Origin:    proto.String("test-origin"),
				Timestamp: proto.Int64(1000000000),
				EventType: events.Envelope_ValueMetric.Enum(),

				// fields that gets sent as tags
				Deployment: proto.String("deployment-name"),
				Job:        proto.String("doppler"),
				Index:      proto.String("1"),
				Ip:         proto.String("10.0.1.2"),

				// additional tags
				Tags: map[string]string{
					"protocol":   "http",
					"request_id": "a1f5-deadbeef",
				},
			})

			var metricPkg []metric.MetricPackage
			Eventually(mchan).Should(Receive(&metricPkg))

			Expect(metricPkg).To(HaveLen(2))
			for _, metric := range metricPkg {
				Expect(metric.MetricValue.Tags).To(Equal([]string{
					"deployment:deployment-name",
					"environment:foo",
					"foundry:bar",
					"index:1",
					"ip:10.0.1.2",
					"job:doppler",
					"name:test-origin",
					"origin:test-origin",
					"protocol:http",
					"request_id:a1f5-deadbeef",
				}))
			}
		})
		// custom tags on app metrics tested in app_metrics_test
		// custom tags on internal metrics tested in datadogclient_test
	})
})

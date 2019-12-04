package util

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Util", func() {
	Context("IsContainerMetric", func() {
		It("correctly identifies a container metric", func() {
			counter := &loggregator_v2.Envelope{
				Timestamp: 1000000000,
				Tags: map[string]string{
					"origin": "origin",
					"deployment": "deployment-name",
					"job": "doppler",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"valueName": &loggregator_v2.GaugeValue{
								Unit: "counter",
								Value: float64(5),
							},
						},
					},
				},
			}

			badGauge := &loggregator_v2.Envelope{
				Timestamp: 1000000000,
				SourceId: "app-id",
				InstanceId: "4",
				Tags: map[string]string{
					"origin": "origin",
					"deployment": "deployment-name",
					"job": "doppler",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(20.0),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							// missing disk
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(29488929),
							},
						},
					},
				},
			}

			badGauge2 := &loggregator_v2.Envelope{
				Timestamp: 1000000000,
				SourceId: "app-id",
				InstanceId: "4",
				Tags: map[string]string{
					"origin": "origin",
					"deployment": "deployment-name",
					"job": "doppler",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(20.0),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk": nil,
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(29488929),
							},
						},
					},
				},
			}

			badGauge3 := &loggregator_v2.Envelope{
				Timestamp: 1000000000,
				SourceId: "app-id",
				InstanceId: "4",
				Tags: map[string]string{
					"origin": "origin",
					"deployment": "deployment-name",
					"job": "doppler",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(20.0),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk": &loggregator_v2.GaugeValue{},
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(29488929),
							},
						},
					},
				},
			}

			goodGauge := &loggregator_v2.Envelope{
				Timestamp: 1000000000,
				SourceId: "app-id",
				InstanceId: "4",
				Tags: map[string]string{
					"origin": "origin",
					"deployment": "deployment-name",
					"job": "doppler",
				},
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(20.0),
							},
							"memory": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(29488929),
							},
							"memory_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(19939949),
							},
							"disk_quota": &loggregator_v2.GaugeValue{
								Unit: "gauge",
								Value: float64(29488929),
							},
						},
					},
				},
			}

			Expect(IsContainerMetric(counter)).To(BeFalse())
			Expect(IsContainerMetric(badGauge)).To(BeFalse())
			Expect(IsContainerMetric(badGauge2)).To(BeFalse())
			Expect(IsContainerMetric(badGauge3)).To(BeFalse())
			Expect(IsContainerMetric(goodGauge)).To(BeTrue())
		})
	})
})
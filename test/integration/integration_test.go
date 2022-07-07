package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"

	datadogclient "github.com/DataDog/datadog-firehose-nozzle/internal/client/datadog"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	. "github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("DatadogFirehoseNozzle", func() {
	var (
		wg             *sync.WaitGroup
		fakeUAA        *FakeUAA
		fakeFirehose   *FakeFirehose
		fakeDatadogAPI *FakeDatadogAPI

		nozzleSession *gexec.Session
	)

	BeforeEach(func() {
		fakeUAA = NewFakeUAA("bearer", "123456789")
		fakeToken := fakeUAA.AuthToken()
		fakeFirehose = NewFakeFirehose(fakeToken)
		fakeDatadogAPI = NewFakeDatadogAPI()

		wg = &sync.WaitGroup{}
		wg.Add(3)

		fakeUAA.Start(wg)
		fakeFirehose.Start(wg)
		fakeDatadogAPI.Start(wg)

		wg.Wait()

		os.Setenv("NOZZLE_FLUSHDURATIONSECONDS", "2")
		os.Setenv("NOZZLE_FLUSHMAXBYTES", "10240")
		os.Setenv("NOZZLE_UAAURL", fakeUAA.URL())
		os.Setenv("NOZZLE_DATADOGURL", fakeDatadogAPI.URL())
		os.Setenv("NOZZLE_RLP_GATEWAY_URL", fakeFirehose.URL())
		os.Setenv("NOZZLE_TRAFFICCONTROLLERURL", strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1))
		os.Setenv("NOZZLE_NUM_WORKERS", "1")
		os.Setenv("NOZZLE_ENVIRONMENT_NAME", "env_name")

		var err error
		nozzleCommand := exec.Command(pathToNozzleExecutable, "-config", "testdata/test-config.json")
		nozzleSession, err = gexec.Start(
			nozzleCommand,
			gexec.NewPrefixedWriter("[o][nozzle] ", GinkgoWriter),
			gexec.NewPrefixedWriter("[e][nozzle] ", GinkgoWriter),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		fakeUAA.Close()
		fakeFirehose.Close()
		fakeDatadogAPI.Close()
		nozzleSession.Kill().Wait()
	})

	It("forwards metrics in a batch", func(done Done) {
		// Give time for the websocket connection to start
		time.Sleep(time.Second)
		fakeFirehose.AddEvent(loggregator_v2.Envelope{
			Timestamp: 1000000000,
			Tags: map[string]string{
				"origin":     "origin",
				"deployment": "deployment-name-aaaaaaaaaaaaaaaaaaaa",
				"job":        "doppler-partition-aaaaaaaaaaaaaaaaaaaa",
				"index":      "1",
			},
			Message: &loggregator_v2.Envelope_Gauge{
				Gauge: &loggregator_v2.Gauge{
					Metrics: map[string]*loggregator_v2.GaugeValue{
						"metricName": &loggregator_v2.GaugeValue{
							Unit:  "gauge",
							Value: float64(5),
						},
					},
				},
			},
		})

		fakeFirehose.AddEvent(loggregator_v2.Envelope{
			Timestamp: 2000000000,
			Tags: map[string]string{
				"origin":     "origin",
				"deployment": "deployment-name-aaaaaaaaaaaaaaaaaaaa",
				"job":        "gorouter-partition-aaaaaaaaaaaaaaaaaaaa",
				"index":      "1",
			},
			Message: &loggregator_v2.Envelope_Gauge{
				Gauge: &loggregator_v2.Gauge{
					Metrics: map[string]*loggregator_v2.GaugeValue{
						"metricName": &loggregator_v2.GaugeValue{
							Unit:  "gauge",
							Value: float64(10),
						},
					},
				},
			},
		})

		fakeFirehose.AddEvent(loggregator_v2.Envelope{
			Timestamp: 3000000000,
			Tags: map[string]string{
				"origin":     "origin",
				"deployment": "deployment-name-aaaaaaaaaaaaaaaaaaaa",
				"job":        "doppler-partition-aaaaaaaaaaaaaaaaaaaa",
			},
			Message: &loggregator_v2.Envelope_Counter{
				Counter: &loggregator_v2.Counter{
					Name:  "counterName",
					Delta: uint64(3),
					Total: uint64(15),
				},
			},
		})

		fakeFirehose.ServeBatch()

		// eventually receive a batch from fake DD
		var messageBytes []byte
		Eventually(fakeDatadogAPI.ReceivedContents, "2s").Should(Receive(&messageBytes))

		// Break JSON blob into a list of blobs, one for each metric
		var payload datadogclient.Payload
		err := json.Unmarshal(Decompress(messageBytes), &payload)
		Expect(err).NotTo(HaveOccurred())

		for _, m := range payload.Series {
			Expect(m.Type).To(Equal("gauge"))

			if m.Metric == "cloudfoundry.nozzle.origin.metricName" || m.Metric == "cloudfoundry.nozzle.metricName" {
				Expect(m.Tags).To(HaveLen(9))
				Expect(m.Tags[0]).To(Equal("deployment:deployment-name"))
				if m.Tags[5] == "job:doppler" {
					Expect(m.Points).To(Equal([]metric.Point{
						{Timestamp: 1, Value: 5.0},
					}))
				} else if m.Tags[5] == "job:gorouter" {
					Expect(m.Points).To(Equal([]metric.Point{
						{Timestamp: 2, Value: 10.0},
					}))
				} else {
					panic("Unknown tag")
				}
			} else if m.Metric == "cloudfoundry.nozzle.origin.counterName" || m.Metric == "cloudfoundry.nozzle.counterName" {
				Expect(m.Tags).To(HaveLen(8))
				Expect(m.Tags[0]).To(Equal("deployment:deployment-name"))
				Expect(m.Tags[1]).To(Equal("deployment:deployment-name-aaaaaaaaaaaaaaaaaaaa"))
				Expect(m.Tags[2]).To(Equal("deployment:deployment-name_env_name"))
				Expect(m.Tags[3]).To(Equal("env:env_name"))
				Expect(m.Tags[4]).To(Equal("job:doppler"))
				Expect(m.Tags[5]).To(Equal("job:doppler-partition-aaaaaaaaaaaaaaaaaaaa"))
				Expect(m.Tags[6]).To(Equal("name:origin"))
				Expect(m.Tags[7]).To(Equal("origin:origin"))

				Expect(m.Points).To(Equal([]metric.Point{
					{Timestamp: 3, Value: 15.0},
				}))
			} else if m.Metric == "cloudfoundry.nozzle.totalMessagesReceived" {
				Expect(m.Tags).To(HaveLen(2))
				Expect(m.Tags[0]).To(HavePrefix("deployment:"))
				Expect(m.Tags[1]).To(HavePrefix("ip:"))

				Expect(m.Points).To(HaveLen(1))
				Expect(m.Points[0].Value).To(Equal(3.0))
			} else if m.Metric == "cloudfoundry.nozzle.totalMetricsSent" {
				Expect(m.Tags).To(HaveLen(2))
				Expect(m.Tags[0]).To(HavePrefix("deployment:"))
				Expect(m.Tags[1]).To(HavePrefix("ip:"))
				Expect(m.Points).To(HaveLen(1))
				Expect(m.Points[0].Value).To(Equal(0.0))
			} else if m.Metric == "cloudfoundry.nozzle.totalLogsSent" {
				Expect(m.Tags).To(HaveLen(2))
				Expect(m.Tags[0]).To(HavePrefix("deployment:"))
				Expect(m.Tags[1]).To(HavePrefix("ip:"))
				Expect(m.Points).To(HaveLen(1))
				Expect(m.Points[0].Value).To(Equal(0.0))
			} else if m.Metric == "cloudfoundry.nozzle.slowConsumerAlert" {

			} else {
				panic("Unknown metric " + m.Metric)
			}
		}

		close(done)
	}, 4.0)
})

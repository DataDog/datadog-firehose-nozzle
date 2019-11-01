package nozzle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/localip"
	"github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/datadog"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/uaatokenfetcher"
	"github.com/DataDog/datadog-firehose-nozzle/test/helper"
)

var _ = Describe("Datadog Firehose Nozzle", func() {
	var (
		fakeUAA        *helper.FakeUAA
		fakeFirehose   *helper.FakeFirehose
		fakeDatadogAPI *helper.FakeDatadogAPI
		fakeCCAPI      *helper.FakeCloudControllerAPI
		configuration  *config.Config
		nozzle         *Nozzle
		log            *gosteno.Logger
		logContent     *bytes.Buffer
		fakeBuffer     *helper.FakeBufferSink
	)

	BeforeEach(func() {
		content := make([]byte, 1024)
		logContent = bytes.NewBuffer(content)
		fakeBuffer = helper.NewFakeBufferSink(logContent)
		c := &gosteno.Config{
			Sinks: []gosteno.Sink{
				fakeBuffer,
			},
		}
		gosteno.Init(c)
		log = gosteno.NewLogger("test")
	})

	Context("regular usage", func() {
		BeforeEach(func() {
			fakeUAA = helper.NewFakeUAA("bearer", "123456789")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = helper.NewFakeFirehose(fakeToken)
			fakeDatadogAPI = helper.NewFakeDatadogAPI()
			fakeCCAPI = helper.NewFakeCloudControllerAPI("bearer", "123456789")
			fakeCCAPI.Start()
			fakeUAA.Start()
			fakeFirehose.Start()
			fakeDatadogAPI.Start()

			configuration = &config.Config{
				UAAURL:               fakeUAA.URL(),
				FlushDurationSeconds: 2,
				FlushMaxBytes:        10240,
				DataDogURL:           fakeDatadogAPI.URL(),
				CloudControllerEndpoint: fakeCCAPI.URL(),
				Client:        			"bearer",
				ClientSecret:      	   "123456789",
				InsecureSSLSkipVerify: true,
				DataDogAPIKey:        "1234567890",
				TrafficControllerURL: strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1),
				DisableAccessControl: false,
				WorkerTimeoutSeconds: 10,
				MetricPrefix:         "datadog.nozzle.",
				Deployment:           "nozzle-deployment",
				AppMetrics:           false,
				NumWorkers:           1,
				OrgDataQuerySeconds:  5,
			}

			tokenFetcher := uaatokenfetcher.New(fakeUAA.URL(), "un", "pwd", true, log)
			nozzle = NewNozzle(configuration, tokenFetcher, log)
			go nozzle.Start()
			time.Sleep(time.Second)
		})

		AfterEach(func() {
			nozzle.Stop()
			fakeUAA.Close()
			fakeFirehose.Close()
			fakeDatadogAPI.Close()
			fakeCCAPI.Close()
		})

		It("receives data from the firehose", func() {
			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(23)) // +3 is because of the internal metrics
		}, 2)

		It("gets a valid authentication token", func() {
			Eventually(fakeFirehose.Requested).Should(BeTrue())
			Consistently(fakeFirehose.LastAuthorization).Should(Equal("bearer 123456789"))
		})

		It("runs orgCollector to obtain org metrics", func() {
			// need to use function to always return the current UsedEndpoints, since it's appended to
			// and thus the address of the slice changes
			Eventually(fakeCCAPI.GetUsedEndpoints, 10, 1).Should(ContainElement("/v2/quota_definitions"))
		})

		It("adds internal metrics and generates aggregate messages when idle", func() {
			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(23))

			validateMetrics(payload, 10, 0)

			// Wait a bit more for the new tick. We should receive only internal metrics
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))
			err = json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(3)) // only internal metrics

			validateMetrics(payload, 10, 23)
		}, 3)

		It("reports a slow-consumer error when the server disconnects abnormally", func() {
			for i := 0; i < 10; i++ {
				envelope := events.Envelope{
					Origin:    proto.String("origin"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_ValueMetric.Enum(),
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", i)),
						Value: proto.Float64(float64(i)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)
			}

			fakeFirehose.SetCloseMessage(websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Client did not respond to ping before keep-alive timeout expired."))
			fakeFirehose.CloseWebSocket()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())

			slowConsumerMetric := findSlowConsumerMetric(payload)
			Expect(slowConsumerMetric).NotTo(BeNil())
			Expect(slowConsumerMetric.Points).To(HaveLen(1))
			Expect(slowConsumerMetric.Points[0].Value).To(BeEquivalentTo(1))

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("Disconnected because nozzle couldn't keep up. Please try scaling up the nozzle."))
			Expect(logOutput).To(ContainSubstring("The Firehose consumer hit a retry error, retrying ..."))
		}, 2)

		It("doesn't report a slow-consumer error when closed for other reasons", func() {
			fakeFirehose.SetCloseMessage(websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, "Weird things happened."))
			fakeFirehose.CloseWebSocket()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())

			slowConsumerMetric := findSlowConsumerMetric(payload)
			Expect(slowConsumerMetric).NotTo(BeNil())
			Expect(slowConsumerMetric.Type).To(Equal("gauge"))
			Expect(slowConsumerMetric.Points).To(HaveLen(1))
			Expect(slowConsumerMetric.Points[0].Value).To(BeEquivalentTo(0))

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("Error while reading from the firehose"))
			Expect(logOutput).NotTo(ContainSubstring("Client did not respond to ping before keep-alive timeout expired."))
			Expect(logOutput).NotTo(ContainSubstring("Disconnected because nozzle couldn't keep up."))
		}, 2)

		Context("receives a truncatingbuffer.droppedmessage value metric", func() {
			It("reports a slow-consumer error", func() {
				slowConsumerError := events.Envelope{
					Origin:    proto.String("doppler"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_CounterEvent.Enum(),
					CounterEvent: &events.CounterEvent{
						Name:  proto.String("TruncatingBuffer.DroppedMessages"),
						Delta: proto.Uint64(1),
						Total: proto.Uint64(1),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(slowConsumerError)

				var contents []byte
				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

				var payload datadog.Payload
				err := json.Unmarshal(helper.Decompress(contents), &payload)
				Expect(err).ToNot(HaveOccurred())

				slowConsumerMetric := findSlowConsumerMetric(payload)
				Expect(slowConsumerMetric).NotTo(BeNil())
				Expect(slowConsumerMetric.Type).To(Equal("gauge"))
				Expect(slowConsumerMetric.Points).To(HaveLen(1))
				Expect(slowConsumerMetric.Points[0].Value).To(BeEquivalentTo(1))
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle."))
			})
		})

		Context("reports a slow-consumer error", func() {
			It("unsets the error after sending it", func() {
				envelope := events.Envelope{
					Origin:    proto.String("doppler"),
					Timestamp: proto.Int64(1000000000),
					EventType: events.Envelope_CounterEvent.Enum(),
					CounterEvent: &events.CounterEvent{
						Name:  proto.String("TruncatingBuffer.DroppedMessages"),
						Delta: proto.Uint64(1),
						Total: proto.Uint64(1),
					},
					ValueMetric: &events.ValueMetric{
						Name:  proto.String(fmt.Sprintf("metricName-%d", 1)),
						Value: proto.Float64(float64(1)),
						Unit:  proto.String("gauge"),
					},
					Deployment: proto.String("deployment-name"),
					Job:        proto.String("doppler"),
				}
				fakeFirehose.AddEvent(envelope)

				var contents []byte
				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

				var payload datadog.Payload
				err := json.Unmarshal(helper.Decompress(contents), &payload)
				Expect(err).ToNot(HaveOccurred())

				slowConsumerMetric := findSlowConsumerMetric(payload)
				Expect(slowConsumerMetric).NotTo(BeNil())
				Expect(slowConsumerMetric.Type).To(Equal("gauge"))
				Expect(slowConsumerMetric.Points).To(HaveLen(1))
				Expect(slowConsumerMetric.Points[0].Value).To(BeEquivalentTo(1))
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle."))

				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))
				err = json.Unmarshal(helper.Decompress(contents), &payload)
				Expect(err).ToNot(HaveOccurred())

				slowConsumerMetric = findSlowConsumerMetric(payload)
				Expect(slowConsumerMetric).NotTo(BeNil())
				Expect(slowConsumerMetric.Type).To(Equal("gauge"))
				Expect(slowConsumerMetric.Points).To(HaveLen(1))
				Expect(slowConsumerMetric.Points[0].Value).To(BeEquivalentTo(0))
			}, 2)
		})

		Context("with DeploymentFilter provided", func() {
			BeforeEach(func() {
				configuration.DeploymentFilter = "good-deployment-name"
			})

			It("includes messages that match deployment filter", func() {
				goodEnvelope := events.Envelope{
					Origin:     proto.String("origin"),
					Timestamp:  proto.Int64(1000000000),
					Deployment: proto.String("good-deployment-name"),
				}
				fakeFirehose.AddEvent(goodEnvelope)
				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive())
			})

			It("filters out messages from other deployments", func() {
				badEnvelope := events.Envelope{
					Origin:     proto.String("origin"),
					Timestamp:  proto.Int64(1000000000),
					Deployment: proto.String("bad-deployment-name"),
				}
				fakeFirehose.AddEvent(badEnvelope)

				rxContents := filterOutNozzleMetrics(configuration.Deployment, fakeDatadogAPI.ReceivedContents)
				Consistently(rxContents, 5*time.Second, time.Second).ShouldNot(Receive())
			})
		})
	})

	Context("when the DisableAccessControl is set to true", func() {
		var tokenFetcher *helper.FakeTokenFetcher

		BeforeEach(func() {
			fakeUAA = helper.NewFakeUAA("", "")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = helper.NewFakeFirehose(fakeToken)
			fakeDatadogAPI = helper.NewFakeDatadogAPI()
			fakeCCAPI = helper.NewFakeCloudControllerAPI("bearer", "123456789")
			tokenFetcher = &helper.FakeTokenFetcher{}

			fakeUAA.Start()
			fakeFirehose.Start()
			fakeDatadogAPI.Start()
			fakeCCAPI.Start()

			configuration = &config.Config{
				FlushDurationSeconds: 1,
				FlushMaxBytes:        10240,
				DataDogURL:           fakeDatadogAPI.URL(),
				DataDogAPIKey:        "1234567890",
				CloudControllerEndpoint: fakeCCAPI.URL(),
				TrafficControllerURL: strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1),
				DisableAccessControl: true,
				NumWorkers:           1,
				AppMetrics:           false,
				OrgDataQuerySeconds:  120,
			}

			tokenFetcher := uaatokenfetcher.New(fakeUAA.URL(), "un", "pwd", true, log)
			nozzle = NewNozzle(configuration, tokenFetcher, log)
			go nozzle.Start()
			time.Sleep(time.Second)
		})

		AfterEach(func() {
			nozzle.Stop()
			fakeUAA.Close()
			fakeFirehose.Close()
			fakeDatadogAPI.Close()
			fakeCCAPI.Close()
		})

		It("can still tries to connect to the firehose", func() {
			Eventually(fakeFirehose.Requested).Should(BeTrue())
		})

		It("gets an empty authentication token", func() {
			Consistently(fakeUAA.Requested).Should(Equal(false))
			Consistently(fakeFirehose.LastAuthorization).Should(Equal(""))
		})

		It("does not require the presence of configuration.UAAURL", func() {
			Consistently(func() int { return tokenFetcher.NumCalls }).Should(Equal(0))
		})
	})

	Context("when workers timeout", func() {
		BeforeEach(func() {
			fakeUAA = helper.NewFakeUAA("bearer", "123456789")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = helper.NewFakeFirehose(fakeToken)
			fakeDatadogAPI = helper.NewFakeDatadogAPI()
			fakeCCAPI = helper.NewFakeCloudControllerAPI("bearer", "123456789")
			fakeUAA.Start()
			fakeFirehose.Start()
			fakeDatadogAPI.Start()
			fakeCCAPI.Start()

			configuration = &config.Config{
				UAAURL:               fakeUAA.URL(),
				FlushDurationSeconds: 2,
				FlushMaxBytes:        10240,
				DataDogURL:           fakeDatadogAPI.URL(),
				DataDogAPIKey:        "1234567890",
				CloudControllerEndpoint: fakeCCAPI.URL(),
				TrafficControllerURL: strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1),
				DisableAccessControl: false,
				WorkerTimeoutSeconds: 1,
				MetricPrefix:         "datadog.nozzle.",
				Deployment:           "nozzle-deployment",
				AppMetrics:           false,
				NumWorkers:           1,
				OrgDataQuerySeconds:  120,
			}

			tokenFetcher := uaatokenfetcher.New(fakeUAA.URL(), "un", "pwd", true, log)
			nozzle = NewNozzle(configuration, tokenFetcher, log)
		})

		AfterEach(func() {
			nozzle.Stop()
			fakeUAA.Close()
			fakeFirehose.Close()
			fakeDatadogAPI.Close()
			fakeCCAPI.Close()
		})

		It("logs a warning", func() {
			go nozzle.Start()
			time.Sleep(time.Second)
			nozzle.workersStopper <- true // Stop one worker
			nozzle.stopWorkers()          // We should hit the worker timeout for one worker

			logOutput := fakeBuffer.GetContent()
			Expect(logOutput).To(ContainSubstring("Could not stop 1 workers after 1s"))
		}, 4)
	})

	Context("without config.CloudControllerEndpoint specified", func() {
		BeforeEach(func() {
			fakeUAA = helper.NewFakeUAA("bearer", "123456789")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = helper.NewFakeFirehose(fakeToken)
			fakeDatadogAPI = helper.NewFakeDatadogAPI()
			fakeUAA.Start()
			fakeFirehose.Start()
			fakeDatadogAPI.Start()

			configuration = &config.Config{
				UAAURL:               fakeUAA.URL(),
				FlushDurationSeconds: 2,
				FlushMaxBytes:        10240,
				DataDogURL:           fakeDatadogAPI.URL(),
				DataDogAPIKey:        "1234567890",
				CloudControllerEndpoint: "",
				TrafficControllerURL: strings.Replace(fakeFirehose.URL(), "http:", "ws:", 1),
				DisableAccessControl: false,
				WorkerTimeoutSeconds: 1,
				MetricPrefix:         "datadog.nozzle.",
				Deployment:           "nozzle-deployment",
				AppMetrics:           false,
				NumWorkers:           1,
				OrgDataQuerySeconds:  120,
			}

			tokenFetcher := uaatokenfetcher.New(fakeUAA.URL(), "un", "pwd", true, log)
			nozzle = NewNozzle(configuration, tokenFetcher, log)
		})

		AfterEach(func() {
			fakeUAA.Close()
			fakeFirehose.Close()
			fakeDatadogAPI.Close()
		})

		It("doesn't fail to start", func() {
			// there are various pieces of nozzle that use config.CloudControllerEndpoint
			// (OrgCollector, AppCache), but we consider them optional, so let's make sure
			// that Nozzle starts when config.CloudControllerEndpoint is nil
			var err error
			go func() {
				err = nozzle.Start()
			}()
			time.Sleep(time.Second)
			nozzle.Stop()
			Expect(err).To(BeNil())
		})
	})
})

func findSlowConsumerMetric(payload datadog.Payload) *metric.Series {
	for _, metric := range payload.Series {
		if metric.Metric == "datadog.nozzle.slowConsumerAlert" {
			return &metric
		}
	}
	return nil
}

func filterOutNozzleMetrics(deployment string, c <-chan []byte) <-chan []byte {
	filter := "deployment:" + deployment
	result := make(chan []byte)
	go func() {
		for b := range c {
			if !strings.Contains(string(helper.Decompress(b)), filter) {
				result <- b
			}
		}
	}()
	return result
}

func validateMetrics(payload datadog.Payload, totalMessagesReceived int, totalMetricsSent int) {
	totalMessagesReceivedFound := false
	totalMetricsSentFound := false
	slowConsumerAlertFound := false
	for _, metric := range payload.Series {
		Expect(metric.Type).To(Equal("gauge"))

		internalMetric := false
		var metricValue int
		if metric.Metric == "datadog.nozzle.totalMessagesReceived" {
			totalMessagesReceivedFound = true
			internalMetric = true
			metricValue = totalMessagesReceived
		}
		if metric.Metric == "datadog.nozzle.totalMetricsSent" {
			totalMetricsSentFound = true
			internalMetric = true
			metricValue = totalMetricsSent
		}
		if metric.Metric == "datadog.nozzle.slowConsumerAlert" {
			slowConsumerAlertFound = true
			internalMetric = true
			metricValue = 0
		}

		if internalMetric {
			Expect(metric.Points).To(HaveLen(1))
			Expect(metric.Points[0].Timestamp).To(BeNumerically(">", time.Now().Unix()-10), "Timestamp should not be less than 10 seconds ago")
			Expect(metric.Points[0].Value).To(Equal(float64(metricValue)))
			ip, _ := localip.LocalIP()
			Expect(metric.Tags).To(Equal([]string{"deployment:nozzle-deployment", "ip:" + ip}))
		}
	}
	Expect(totalMessagesReceivedFound).To(BeTrue())
	Expect(totalMetricsSentFound).To(BeTrue())
	Expect(slowConsumerAlertFound).To(BeTrue())
}

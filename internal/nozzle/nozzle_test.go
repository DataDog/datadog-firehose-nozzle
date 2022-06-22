package nozzle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/localip"
	"github.com/cloudfoundry/gosteno"
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
		fakeDCAAPI     *helper.FakeClusterAgentAPI
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
			time.Sleep(time.Second)

			configuration = &config.Config{
				UAAURL:                    fakeUAA.URL(),
				FlushDurationSeconds:      2,
				FlushMaxBytes:             10240,
				DataDogURL:                fakeDatadogAPI.URL(),
				CloudControllerEndpoint:   fakeCCAPI.URL(),
				RLPGatewayURL:             fakeFirehose.URL(),
				Client:                    "bearer",
				ClientSecret:              "123456789",
				InsecureSSLSkipVerify:     true,
				DataDogAPIKey:             "1234567890",
				DisableAccessControl:      false,
				WorkerTimeoutSeconds:      10,
				MetricPrefix:              "datadog.nozzle.",
				Deployment:                "nozzle-deployment",
				AppMetrics:                false,
				NumWorkers:                1,
				OrgDataCollectionInterval: 5,
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
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "deployment-name",
						"job":        "doppler",
					},
					Message: &loggregator_v2.Envelope_Gauge{
						Gauge: &loggregator_v2.Gauge{
							Metrics: map[string]*loggregator_v2.GaugeValue{
								fmt.Sprintf("metricName-%d", i): &loggregator_v2.GaugeValue{
									Unit:  "counter",
									Value: float64(i),
								},
							},
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				// stream a batch in the middle to test multiple envelope batches
				if i == 4 {
					fakeFirehose.ServeBatch()
				}
			}
			fakeFirehose.ServeBatch()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(25)) // +3 is because of the internal metrics, +2 because of org metrics
		}, 2)

		It("gets a valid authentication token", func() {
			Eventually(fakeFirehose.Requested, 10).Should(BeTrue())
			Consistently(fakeFirehose.LastAuthorization).Should(Equal("bearer 123456789"))
		})

		It("refreshes authentication token when expired", func() {
			Eventually(fakeUAA.Requested, 10).Should(BeTrue())
			Eventually(fakeFirehose.Requested).Should(BeTrue())

			fakeFirehose.SetToken("invalid")
			fakeFirehose.CloseServerLoop()
			fakeFirehose.SetToken("123456789")

			Eventually(fakeUAA.TimesRequested).Should(BeNumerically(">", 1))

			fakeFirehose.ServeBatch()
			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))
		})

		It("runs orgCollector to obtain org metrics", func() {
			// need to use function to always return the current UsedEndpoints, since it's appended to
			// and thus the address of the slice changes
			Eventually(fakeCCAPI.GetUsedEndpoints, 10, 1).Should(ContainElement("/v2/quota_definitions"))
		})

		It("adds internal metrics and generates aggregate messages when idle", func() {
			for i := 0; i < 10; i++ {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "deployment-name",
						"job":        "doppler",
					},
					Message: &loggregator_v2.Envelope_Gauge{
						Gauge: &loggregator_v2.Gauge{
							Metrics: map[string]*loggregator_v2.GaugeValue{
								fmt.Sprintf("metricName-%d", i): &loggregator_v2.GaugeValue{
									Unit:  "counter",
									Value: float64(i),
								},
							},
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
			}
			fakeFirehose.ServeBatch()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(25))

			validateMetrics(payload, 11, 0, 0, 0) // +1 for total messages because of Org Quota

			// Wait a bit more for the new tick. We should receive only internal metrics
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))
			err = json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(5)) // only internal metrics

			validateMetrics(payload, 11, 25, 25, 0)
		}, 3)

		Context("receives a rlp.dropped value metric", func() {
			It("reports a slow-consumer error", func() {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":    "loggregator.rlp",
						"direction": "egress",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Delta: uint64(1),
							Total: uint64(2),
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				fakeFirehose.ServeBatch()

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
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("nozzle is not keeping up"))
			})
		})

		Context("reports a slow-consumer error", func() {
			It("unsets the error after sending it", func() {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":    "loggregator.rlp",
						"direction": "egress",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Delta: uint64(1),
							Total: uint64(2),
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				fakeFirehose.ServeBatch()

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
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("nozzle is not keeping up"))

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
				goodEnvelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "good-deployment-name",
					},
				}
				fakeFirehose.AddEvent(goodEnvelope)
				fakeFirehose.ServeBatch()
				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive())
			})

			It("filters out messages from other deployments", func() {
				badEnvelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "bad-deployment-name",
					},
				}
				fakeFirehose.AddEvent(badEnvelope)
				fakeFirehose.ServeBatch()

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
				FlushDurationSeconds:      1,
				FlushMaxBytes:             10240,
				DataDogURL:                fakeDatadogAPI.URL(),
				DataDogAPIKey:             "1234567890",
				CloudControllerEndpoint:   fakeCCAPI.URL(),
				RLPGatewayURL:             fakeFirehose.URL(),
				DisableAccessControl:      true,
				NumWorkers:                1,
				AppMetrics:                false,
				OrgDataCollectionInterval: 120,
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
				UAAURL:                    fakeUAA.URL(),
				FlushDurationSeconds:      2,
				FlushMaxBytes:             10240,
				DataDogURL:                fakeDatadogAPI.URL(),
				DataDogAPIKey:             "1234567890",
				CloudControllerEndpoint:   fakeCCAPI.URL(),
				RLPGatewayURL:             fakeFirehose.URL(),
				DisableAccessControl:      false,
				WorkerTimeoutSeconds:      1,
				MetricPrefix:              "datadog.nozzle.",
				Deployment:                "nozzle-deployment",
				AppMetrics:                false,
				NumWorkers:                1,
				OrgDataCollectionInterval: 120,
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
			time.Sleep(5 * time.Second)
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
				UAAURL:                    fakeUAA.URL(),
				FlushDurationSeconds:      2,
				FlushMaxBytes:             10240,
				DataDogURL:                fakeDatadogAPI.URL(),
				DataDogAPIKey:             "1234567890",
				CloudControllerEndpoint:   "",
				RLPGatewayURL:             fakeFirehose.URL(),
				DisableAccessControl:      false,
				WorkerTimeoutSeconds:      1,
				MetricPrefix:              "datadog.nozzle.",
				Deployment:                "nozzle-deployment",
				AppMetrics:                false,
				NumWorkers:                1,
				OrgDataCollectionInterval: 120,
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

	Context("regular usage with dca client", func() {
		BeforeEach(func() {
			fakeUAA = helper.NewFakeUAA("bearer", "123456789")
			fakeToken := fakeUAA.AuthToken()
			fakeFirehose = helper.NewFakeFirehose(fakeToken)
			fakeDatadogAPI = helper.NewFakeDatadogAPI()
			fakeCCAPI = helper.NewFakeCloudControllerAPI("bearer", "123456789")
			fakeDCAAPI = helper.NewFakeClusterAgentAPI("bearer", "123456789")
			fakeCCAPI.Start()
			fakeDCAAPI.Start()
			fakeUAA.Start()
			fakeFirehose.Start()
			fakeDatadogAPI.Start()

			configuration = &config.Config{
				UAAURL:                    fakeUAA.URL(),
				FlushDurationSeconds:      2,
				FlushMaxBytes:             10240,
				DataDogURL:                fakeDatadogAPI.URL(),
				CloudControllerEndpoint:   fakeCCAPI.URL(),
				RLPGatewayURL:             fakeFirehose.URL(),
				Client:                    "bearer",
				ClientSecret:              "123456789",
				InsecureSSLSkipVerify:     true,
				DataDogAPIKey:             "1234567890",
				DisableAccessControl:      false,
				WorkerTimeoutSeconds:      10,
				MetricPrefix:              "datadog.nozzle.",
				Deployment:                "nozzle-deployment",
				AppMetrics:                false,
				NumWorkers:                1,
				OrgDataCollectionInterval: 5,
				DCAEnabled:                true,
				DCAUrl:                    fakeDCAAPI.URL(),
				DCAToken:                  "123456789",
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
			fakeDCAAPI.Close()
			fakeCCAPI.Close()
		})

		It("receives data from the firehose", func() {
			for i := 0; i < 10; i++ {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "deployment-name",
						"job":        "doppler",
					},
					Message: &loggregator_v2.Envelope_Gauge{
						Gauge: &loggregator_v2.Gauge{
							Metrics: map[string]*loggregator_v2.GaugeValue{
								fmt.Sprintf("metricName-%d", i): &loggregator_v2.GaugeValue{
									Unit:  "counter",
									Value: float64(i),
								},
							},
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				// stream a batch in the middle to test multiple envelope batches
				if i == 4 {
					fakeFirehose.ServeBatch()
				}
			}
			fakeFirehose.ServeBatch()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(25)) // +3 is because of the internal metrics, +2 because of org metrics
		}, 2)

		It("gets a valid authentication token", func() {
			Eventually(fakeFirehose.Requested, 10).Should(BeTrue())
			Consistently(fakeFirehose.LastAuthorization).Should(Equal("bearer 123456789"))
		})

		It("runs orgCollector to obtain org metrics", func() {
			// need to use function to always return the current UsedEndpoints, since it's appended to
			// and thus the address of the slice changes
			Eventually(fakeDCAAPI.GetUsedEndpoints, 10, 1).Should(ContainElement("/api/v1/cf/org_quotas"))
		})

		It("adds internal metrics and generates aggregate messages when idle", func() {
			for i := 0; i < 10; i++ {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "deployment-name",
						"job":        "doppler",
					},
					Message: &loggregator_v2.Envelope_Gauge{
						Gauge: &loggregator_v2.Gauge{
							Metrics: map[string]*loggregator_v2.GaugeValue{
								fmt.Sprintf("metricName-%d", i): &loggregator_v2.GaugeValue{
									Unit:  "counter",
									Value: float64(i),
								},
							},
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
			}
			fakeFirehose.ServeBatch()

			var contents []byte
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))

			var payload datadog.Payload
			err := json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(25))

			validateMetrics(payload, 11, 0, 0, 0) // +1 for total messages because of Org Quota

			// Wait a bit more for the new tick. We should receive only internal metrics
			Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive(&contents))
			err = json.Unmarshal(helper.Decompress(contents), &payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(payload.Series).To(HaveLen(5)) // only internal metrics

			validateMetrics(payload, 11, 25, 25, 0)
		}, 3)

		Context("receives a rlp.dropped value metric", func() {
			It("reports a slow-consumer error", func() {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":    "loggregator.rlp",
						"direction": "egress",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Delta: uint64(1),
							Total: uint64(2),
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				fakeFirehose.ServeBatch()

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
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("nozzle is not keeping up"))
			})
		})

		Context("reports a slow-consumer error", func() {
			It("unsets the error after sending it", func() {
				envelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":    "loggregator.rlp",
						"direction": "egress",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Delta: uint64(1),
							Total: uint64(2),
						},
					},
				}
				fakeFirehose.AddEvent(envelope)
				fakeFirehose.ServeBatch()

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
				Expect(fakeBuffer.GetContent()).To(ContainSubstring("nozzle is not keeping up"))

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
				goodEnvelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "good-deployment-name",
					},
				}
				fakeFirehose.AddEvent(goodEnvelope)
				fakeFirehose.ServeBatch()
				Eventually(fakeDatadogAPI.ReceivedContents, 15*time.Second, time.Second).Should(Receive())
			})

			It("filters out messages from other deployments", func() {
				badEnvelope := loggregator_v2.Envelope{
					Timestamp: 1000000000,
					Tags: map[string]string{
						"origin":     "origin",
						"deployment": "bad-deployment-name",
					},
				}
				fakeFirehose.AddEvent(badEnvelope)
				fakeFirehose.ServeBatch()

				rxContents := filterOutNozzleMetrics(configuration.Deployment, fakeDatadogAPI.ReceivedContents)
				Consistently(rxContents, 5*time.Second, time.Second).ShouldNot(Receive())
			})
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

func validateMetrics(payload datadog.Payload, totalMessagesReceived, totalMetricsSent, metricsSent, metricsDropped int) {
	totalMessagesReceivedFound := false
	totalMetricsSentFound := false
	slowConsumerAlertFound := false

	for _, metric := range payload.Series {
		internalMetric := false
		var metricValue int
		if metric.Metric == "datadog.nozzle.totalMessagesReceived" {
			Expect(metric.Type).To(Equal("gauge"))
			totalMessagesReceivedFound = true
			internalMetric = true
			metricValue = totalMessagesReceived
		}
		if metric.Metric == "datadog.nozzle.totalMetricsSent" {
			Expect(metric.Type).To(Equal("gauge"))
			totalMetricsSentFound = true
			internalMetric = true
			metricValue = totalMetricsSent
		}
		if metric.Metric == "datadog.nozzle.slowConsumerAlert" {
			Expect(metric.Type).To(Equal("gauge"))
			slowConsumerAlertFound = true
			internalMetric = true
			metricValue = 0
		}
		if metric.Metric == "datadog.nozzle.metrics.sent" {
			Expect(metric.Type).To(Equal("count"))
			internalMetric = true
			metricValue = metricsSent
		}
		if metric.Metric == "datadog.nozzle.metrics.dropped" {
			Expect(metric.Type).To(Equal("count"))
			internalMetric = true
			metricValue = metricsDropped
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

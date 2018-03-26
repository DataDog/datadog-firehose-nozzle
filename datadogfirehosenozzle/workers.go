package datadogfirehosenozzle

import (
	"github.com/DataDog/datadog-firehose-nozzle/datadogfirehosenozzle/nozzlestats"
	"github.com/cloudfoundry/sonde-go/events"
)

func (d *DatadogFirehoseNozzle) startWorkers() {
	// Start the (multiple) workers which will process envelopes
	for i := 0; i < d.config.NumWorkers; i++ {
		go d.work()
	}

	// Start the (one) worker which will store metrics as they're generated
	go d.readProcessedMetrics()
}

func (d *DatadogFirehoseNozzle) stopWorkers() {
	go func() {
		for i := 0; i < d.config.NumWorkers+1; i++ {
			// +1 is for the readProcessedMetrics worker
			d.workersStopper <- true
		}
	}()
}

func (d *DatadogFirehoseNozzle) work() {
	for {
		select {
		case envelope := <-d.messages:
			if !d.keepMessage(envelope) {
				continue
			}
			d.handleMessage(envelope)
			d.processor.ProcessMetric(envelope)
		case <-d.workersStopper:
			return
		}
	}
}

func (d *DatadogFirehoseNozzle) readProcessedMetrics() {
	for {
		select {
		case pkg := <-d.processedMetrics:
			d.mapLock.Lock()
			nozzlestats.TotalMessagesReceived.Add(1)
			for _, m := range pkg {
				d.metricsMap.Add(*m.MetricKey, *m.MetricValue)
			}
			d.mapLock.Unlock()
		case <-d.workersStopper:
			return
		}
	}
}

func (d *DatadogFirehoseNozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		d.log.Infof("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		d.AlertSlowConsumerError()
	}
}

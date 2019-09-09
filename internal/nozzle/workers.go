package nozzle

import (
	"time"

	"github.com/cloudfoundry/sonde-go/events"
)

func (d *Nozzle) startWorkers() {
	// Start the (multiple) workers which will process envelopes,
	// create metricPackages and send them to p.processedMetrics channel
	// NOTE: Worker are used to process infra or app event envelopes to metricPackages
	d.log.Infof("Starting processed metrics reader and %d workers...", d.config.NumWorkers)
	for i := 0; i < d.config.NumWorkers; i++ {
		go d.work()
	}

	// Start the (one) worker which will read from the p.processedMetrics channel and store metrics as they're generated
	// into d.metricsMap
	// NOTE: This step is not parallelized and should part of another class
	go d.readProcessedMetrics()
}

func (d *Nozzle) stopWorkers() {
	// Stop the app metrics cache refreshing loop if it's started
	d.processor.StopAppMetrics()

	timedOut := false
	for i := 0; i < d.config.NumWorkers+1; i++ {
		// +1 is for the readProcessedMetrics worker
		select {
		case d.workersStopper <- true:
		case <-time.After(time.Duration(d.config.WorkerTimeoutSeconds) * time.Second):
			// No worker responded in time to get the stop message
			// Assuming they crashed
			d.log.Warnf("Could not stop %d workers after %ds", d.config.NumWorkers+1-i, d.config.WorkerTimeoutSeconds)
			timedOut = true
		}
		if timedOut {
			break
		}
	}
}

func (d *Nozzle) work() {
	d.log.Info("Worker started")
	for {
		select {
		case envelope := <-d.messages:
			if !d.keepMessage(envelope) {
				continue
			}
			d.handleMessage(envelope)
			d.processor.ProcessMetric(envelope)
		case <-d.workersStopper:
			d.log.Info("Worker shutting down...")
			return
		}
	}
}

func (d *Nozzle) readProcessedMetrics() {
	d.log.Info("Processed metrics reader started")
	for {
		select {
		case pkg := <-d.processedMetrics:
			d.mapLock.Lock()
			d.totalMessagesReceived++
			for _, m := range pkg {
				d.metricsMap.Add(*m.MetricKey, *m.MetricValue)
			}
			d.mapLock.Unlock()
		case <-d.workersStopper:
			d.log.Info("Processed metrics reader shutting down...")
			return
		}
	}
}

func (d *Nozzle) handleMessage(envelope *events.Envelope) {
	if envelope.GetEventType() == events.Envelope_CounterEvent && envelope.CounterEvent.GetName() == "TruncatingBuffer.DroppedMessages" && envelope.GetOrigin() == "doppler" {
		d.log.Infof("We've intercepted an upstream message which indicates that the nozzle or the TrafficController is not keeping up. Please try scaling up the nozzle.")
		d.AlertSlowConsumerError()
	}
}

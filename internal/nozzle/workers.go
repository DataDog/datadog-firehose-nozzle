package nozzle

import (
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
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

	if d.config.EnableApplicationLogs {
		go d.readProcessedLogs()
	}

}

func (d *Nozzle) stopWorkers() {
	// Stop the app metrics cache refreshing loop if it's started
	d.processor.StopAppMetrics()

	timedOut := false

	// +1 is for the readProcessedMetrics worker
	numWorkers := d.config.NumWorkers + 1

	// +1 for the readProcessedLogs worker
	if d.config.DataDogLogIntakeURL != "" || len(d.config.DataDogAdditionalLogIntakeEndpoints) > 0 {
		numWorkers++
	}
	for i := 0; i < numWorkers; i++ {
		select {
		case d.workersStopper <- true:
		case <-time.After(time.Duration(d.config.WorkerTimeoutSeconds) * time.Second):
			// No worker responded in time to get the stop message
			// Assuming they crashed
			d.log.Warnf("Could not stop %d workers after %ds", numWorkers-i, d.config.WorkerTimeoutSeconds)
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

			// logs
			if d.config.EnableApplicationLogs {
				if l := envelope.GetLog(); l != nil {
					d.processor.ProcessLog(envelope)
					continue
				}
			}

			// metrics
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

func (d *Nozzle) readProcessedLogs() {
	d.log.Info("Processed logs reader started")
	for {
		select {
		case logMessage := <-d.processedLogs:
			d.mapLock.Lock()
			d.totalMessagesReceived++
			d.logsBuffer = append(d.logsBuffer, logMessage)
			d.mapLock.Unlock()
		case <-d.workersStopper:
			d.log.Info("Processed logs reader shutting down...")
			return
		}
	}
}

func (d *Nozzle) handleMessage(envelope *loggregator_v2.Envelope) {
	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Counter:
		// TODO: make sure this is correct (docs at https://docs.pivotal.io/platform/2-7/monitoring/key-cap-scaling.html)
		if envelope.GetCounter().GetName() == "dropped" {
			origin, ook := envelope.GetTags()["origin"]
			direction, dok := envelope.GetTags()["direction"]
			if ook && origin == "loggregator.rlp" && dok && direction == "egress" && envelope.GetCounter().GetDelta() > 0 {
				d.log.Infof("We've intercepted an upstream message which indicates that the nozzle is not keeping up. Please try scaling up the nozzle.")
				d.AlertSlowConsumerError()
			}
		}
	}
}

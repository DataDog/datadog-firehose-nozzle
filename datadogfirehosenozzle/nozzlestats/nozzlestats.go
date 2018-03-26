package nozzlestats

import (
	"expvar"
)

var (
	// TotalMessagesReceived is the total number of messages recieved.
	// it's modified by workers and read by the main thread
	TotalMessagesReceived *expvar.Int
	// TotalMetricsSent is the total number of metrics sent
	// It's read by the main thread and modified by workers
	TotalMetricsSent *expvar.Int
)

// This package is just for ease of access of these metrics
// And so that there does not need to be manual management of access to these metrics

func init() {
	TotalMessagesReceived = expvar.NewInt("totalMessagesReceived")
	TotalMetricsSent = expvar.NewInt("totalMetricsSent")

	TotalMessagesReceived.Set(0)
	TotalMetricsSent.Set(0)
}

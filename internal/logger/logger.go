package logger

import (
	"os"
	"strings"

	"github.com/cloudfoundry/gosteno"
)

func NewLogger(verbose bool, logFilePath, name string, syslogNamespace string) *gosteno.Logger {
	level := gosteno.LOG_INFO

	if verbose {
		level = gosteno.LOG_DEBUG
	}

	loggingConfig := &gosteno.Config{
		Sinks:     make([]gosteno.Sink, 1),
		Level:     level,
		Codec:     gosteno.NewJsonCodec(),
		EnableLOC: true}

	if strings.TrimSpace(logFilePath) == "" {
		loggingConfig.Sinks[0] = gosteno.NewIOSink(os.Stdout)
	} else {
		loggingConfig.Sinks[0] = gosteno.NewFileSink(logFilePath)
	}

	if syslogNamespace != "" {
		loggingConfig.Sinks = append(loggingConfig.Sinks, GetNewSyslogSink(syslogNamespace))
	}

	gosteno.Init(loggingConfig)
	logger := gosteno.NewLogger(name)
	logger.Debugf("Component %s in debug mode!", name)

	return logger
}

type RLPLogForwarder struct {
	// The loggregator.WithRLPGatewayClientLogger only takes log.Logger as argument,
	// so this forwarder serves as it's "output" to proxy logs to our gosteno.Logger
	log *gosteno.Logger
}

func NewRLPLogForwarder(gostenoLog *gosteno.Logger) *RLPLogForwarder {
	return &RLPLogForwarder{
		log: gostenoLog,
	}
}

func (f RLPLogForwarder) Write(p []byte) (n int, err error) {
	message := "message forwarded from RLP client: " + string(p)
	f.log.Debug(message)
	return len(p), nil
}

package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/logger"
	"github.com/DataDog/datadog-firehose-nozzle/internal/nozzle"
	"github.com/DataDog/datadog-firehose-nozzle/internal/uaatokenfetcher"
)

const flushMinBytes uint32 = 1024

var (
	logFilePath = flag.String("logFile", "", "The agent log file, defaults to STDOUT")
	logLevel    = flag.Bool("debug", false, "Debug logging")
	configFile  = flag.String("config", "config/datadog-firehose-nozzle.json", "Location of the nozzle config json file")
)

func main() {
	flag.Parse()
	// Initialize logger
	log := logger.NewLogger(*logLevel, *logFilePath, "datadog-firehose-nozzle", "")

	// Load Nozzle Config
	var err error
	config.NozzleConfig, err = config.Parse(*configFile)
	if err != nil {
		log.Fatalf("Error parsing config: %s", err.Error())
	}
	if config.NozzleConfig.FlushMaxBytes < flushMinBytes {
		log.Fatalf("Config FlushMaxBytes is too low (%d): must be at least %d", config.NozzleConfig.FlushMaxBytes, flushMinBytes)
	}
	logString, err := config.NozzleConfig.AsLogString()
	if err != nil {
		log.Warnf("Failed to serialize config for logging: %s", err.Error())
	} else {
		log.Infof("Running nozzle with following config: %s", logString)
	}
	// Initialize UAATokenFetcher
	tokenFetcher := uaatokenfetcher.New(
		config.NozzleConfig.UAAURL,
		config.NozzleConfig.Client,
		config.NozzleConfig.ClientSecret,
		config.NozzleConfig.InsecureSSLSkipVerify,
		log,
	)

	threadDumpChan := registerGoRoutineDumpSignalChannel()
	defer close(threadDumpChan)
	go dumpGoRoutine(threadDumpChan)

	// Initialize and start Nozzle
	log.Infof("Targeting datadog API URL: %s \n", config.NozzleConfig.DataDogURL)
	log.Infof("Targeting datadog LogIntake URL: %s \n", config.NozzleConfig.DataDogLogIntakeURL)
	datadogNozzle := nozzle.NewNozzle(&config.NozzleConfig, tokenFetcher, log)
	err = datadogNozzle.Start()
	if err != nil {
		log.Error(err.Error())
	}
}

func registerGoRoutineDumpSignalChannel() chan os.Signal {
	threadDumpChan := make(chan os.Signal, 1)
	signal.Notify(threadDumpChan, syscall.SIGUSR1)

	return threadDumpChan
}

func dumpGoRoutine(dumpChan chan os.Signal) {
	for range dumpChan {
		goRoutineProfiles := pprof.Lookup("goroutine")
		if goRoutineProfiles != nil {
			_ = goRoutineProfiles.WriteTo(os.Stdout, 2)
		}
	}
}

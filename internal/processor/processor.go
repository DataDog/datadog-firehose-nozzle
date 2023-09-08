package processor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/logs"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/processor/parser"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/cloudfoundry/gosteno"
)

const (
	deploymentUUIDPattern   = "-([0-9a-f]{20})"
	jobPartitionUUIDPattern = "-partition-([0-9a-f]{20})"
	enableLogsTagKey        = "dd_enable_logs"
)

// Processor extracts metrics from envelopes
type Processor struct {
	processedMetrics      chan<- []metric.MetricPackage
	processedLogs         chan<- logs.LogMessage
	appMetrics            parser.Parser
	appCache              *parser.AppCache
	customTags            []string
	environment           string
	deploymentUUIDRegex   *regexp.Regexp
	jobPartitionUUIDRegex *regexp.Regexp
	log                   *gosteno.Logger
}

// NewProcessor creates a new processor
func NewProcessor(
	pm chan<- []metric.MetricPackage,
	pl chan<- logs.LogMessage,
	customTags []string,
	environment string,
	parseAppMetricsEnable bool,
	cfClient *cloudfoundry.CFClient,
	dcaClient *cloudfoundry.DCAClient,
	numCacheWorkers int,
	grabInterval int,
	log *gosteno.Logger,
) (*Processor, bool) {

	processor := &Processor{
		processedMetrics:      pm,
		processedLogs:         pl,
		appCache:              parser.GetGlobalAppCache(),
		customTags:            customTags,
		environment:           environment,
		deploymentUUIDRegex:   regexp.MustCompile(deploymentUUIDPattern),
		jobPartitionUUIDRegex: regexp.MustCompile(jobPartitionUUIDPattern),
		log:                   log,
	}

	if parseAppMetricsEnable {
		appMetrics, err := parser.NewAppParser(
			cfClient,
			dcaClient,
			numCacheWorkers,
			grabInterval,
			log,
			customTags,
			environment,
			true,
		)
		if err != nil {
			parseAppMetricsEnable = false
			log.Warnf("error setting up appMetrics, continuing without application metrics: %v", err)
		} else {
			log.Debug("setting up app metrics")
			processor.appMetrics = appMetrics
		}
	}

	return processor, parseAppMetricsEnable
}

// ProcessMetric takes an envelope, parses it and sends the processed metrics to the nozzle
func (p *Processor) ProcessMetric(envelope *loggregator_v2.Envelope) {
	var err error
	var metricsPackages []metric.MetricPackage

	// Parse infrastructure type of envelopes
	infraParser, _ := parser.NewInfraParser(
		p.environment,
		p.deploymentUUIDRegex,
		p.jobPartitionUUIDRegex,
		p.customTags,
	)
	metricsPackages, err = infraParser.ParseMetrics(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
		// it can only be one or the other
		return
	}

	// Parse application type of envelopes
	metricsPackages, err = p.parseAppMetric(envelope)
	if err == nil {
		p.processedMetrics <- metricsPackages
	}
}

// ProcessLog takes an envelope, parses it and sends the processed logs to the nozzle
func (p *Processor) ProcessLog(envelope *loggregator_v2.Envelope) {
	var err error
	var logsMessage logs.LogMessage

	var appTags []string
	var serviceName string = envelope.SourceId
	var source string

	cfapp := p.appCache.Get(envelope.SourceId)
	if cfapp != nil {
		for _, tag := range cfapp.Tags {
			// check if app is enabling logs collection
			if strings.HasPrefix(tag, enableLogsTagKey) {
				if strings.Contains(tag, "false") {
					p.log.Debugf("skipped log envelope for app %s", cfapp.Name)
					return
				}
				break
			}
		}
		appTags = cfapp.Tags
		appTags = append(appTags, fmt.Sprintf("application_id:%s", envelope.GetTags()["app_id"]))
		appTags = append(appTags, fmt.Sprintf("application_name:%s", envelope.GetTags()["app_name"]))
		appTags = append(appTags, fmt.Sprintf("instance_index:%s", envelope.GetTags()["instance_id"]))
		serviceName = cfapp.Name
	}

	// Parse infrastructure type of envelopes
	infraParser, _ := parser.NewInfraParser(
		p.environment,
		p.deploymentUUIDRegex,
		p.jobPartitionUUIDRegex,
		append(p.customTags, appTags...),
	)

	logsMessage, err = infraParser.ParseLog(envelope)
	logsMessage.Service = serviceName

	// Detect source
	if job, ok := envelope.GetTags()["job"]; ok {
		source = job
	} else if source == "" {
		source = "datadog-firehose-nozzle"
	}

	logsMessage.Source = source

	if err == nil {
		p.processedLogs <- logsMessage
		return
	}
}

// StopAppMetrics stops the goroutine refreshing the apps cache
func (p *Processor) StopAppMetrics() {
	if p.appMetrics == nil {
		return
	}

	appParser := p.appMetrics.(*parser.AppParser)
	appParser.Stop()
}

func (p *Processor) parseAppMetric(envelope *loggregator_v2.Envelope) ([]metric.MetricPackage, error) {
	var metricsPackages []metric.MetricPackage
	var err error

	if p.appMetrics == nil {
		return metricsPackages, fmt.Errorf("app metrics are not configured")
	}

	if !util.IsContainerMetric(envelope) {
		return metricsPackages, fmt.Errorf("not an app metric")
	}

	appParser := p.appMetrics.(*parser.AppParser)
	if !appParser.AppCache.IsWarmedUp() {
		return metricsPackages, fmt.Errorf("app metrics cache is not yet ready, skipping envelope")
	}

	metricsPackages, err = p.appMetrics.Parse(envelope)

	return metricsPackages, err
}

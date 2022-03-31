// NOTE: whenever the CC API V3 exposes the quota information for us,
// we should just store everything in the app cache and get it from there
package orgcollector

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/internal/client/cloudfoundry"
	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/DataDog/datadog-firehose-nozzle/internal/util"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/gosteno"
)

type OrgCollector struct {
	cfClient         *cloudfoundry.CFClient
	dcaClient        *cloudfoundry.DCAClient
	log              *gosteno.Logger
	processedMetrics chan<- []metric.MetricPackage
	customTags       []string
	queryInterval    uint32
	stopper          chan bool
}

func NewOrgCollector(
	config *config.Config,
	processedMetrics chan<- []metric.MetricPackage,
	log *gosteno.Logger,
	customTags []string) (*OrgCollector, error) {
	var cfClient *cloudfoundry.CFClient
	var dcaClient *cloudfoundry.DCAClient
	var err error

	if config.DCAEnabled {
		dcaClient, err = cloudfoundry.NewDCAClient(config, log)
	} else {
		cfClient, err = cloudfoundry.NewClient(config, log)
	}

	if err != nil {
		return nil, err
	}

	return &OrgCollector{
		cfClient:         cfClient,
		dcaClient:        dcaClient,
		log:              log,
		processedMetrics: processedMetrics,
		customTags:       customTags,
		queryInterval:    config.OrgDataCollectionInterval,
		stopper:          make(chan bool),
	}, nil
}

func (o *OrgCollector) Start() {
	go o.run()
}

func (o *OrgCollector) Stop() {
	o.stopper <- true
}

func (o *OrgCollector) run() {
	// Query the data with random jitter so that we don't overload the cloud controller
	ticker, jitterWait := util.GetTickerWithJitter(o.queryInterval, 0.1)
	defer ticker.Stop()
	// If the nozzle is restarted in the middle of o.queryInterval, there'd
	// be a quite large gap in the data submitted
	go o.pushMetrics()
	for {
		select {
		case <-ticker.C:
			jitterWait()
			go o.pushMetrics()
		case <-o.stopper:
			return
		}
	}
}

func (o *OrgCollector) pushMetrics() {
	o.log.Info("Collecting org quotas ...")
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	// Fetch orgs
	wg.Add(1)
	var allOrgs []cfclient.V3Organization
	go func() {
		defer wg.Done()
		var err error
		if o.dcaClient != nil {
			allOrgs, err = o.dcaClient.GetV3Orgs()
		} else {
			allOrgs, err = o.cfClient.GetV3Orgs()
		}

		if err != nil {
			errors <- err
		}
	}()

	// Fetch quotas
	wg.Add(1)
	var allQuotas []cloudfoundry.CFOrgQuota
	go func() {
		defer wg.Done()
		var err error
		if o.dcaClient != nil {
			allQuotas, err = o.dcaClient.GetV2OrgQuotas()
		} else {
			allQuotas, err = o.cfClient.GetV2OrgQuotas()
		}
		if err != nil {
			errors <- err
		}
	}()

	wg.Wait()
	close(errors)

	// Go through the channel and print all errors
	for err := range errors {
		o.log.Error(err.Error())
	}

	// Create a map of {OrgQuota.Guid: OrgQuota} to make access fast
	quotaGuidsToObjects := map[string]cloudfoundry.CFOrgQuota{}
	for _, q := range allQuotas {
		quotaGuidsToObjects[q.GUID] = q
	}

	metricsPackages := []metric.MetricPackage{}
	for _, org := range allOrgs {
		q, ok := quotaGuidsToObjects[org.Relationships["quota"].Data.GUID]
		if !ok {
			o.log.Warnf("failed to get quota for org %s", org.GUID)
			continue
		}
		tags := []string{}
		tags = append(tags, o.customTags...)
		tags = append(tags, o.getTagsFromOrg(org)...)
		key := metric.MetricKey{
			Name:     "org.memory.quota",
			TagsHash: util.HashTags(tags),
		}
		value := metric.MetricValue{
			Tags: tags,
			Points: []metric.Point{
				metric.Point{
					Timestamp: time.Now().Unix(),
					Value:     float64(q.MemoryLimit),
				},
			},
		}
		metricsPackages = append(metricsPackages, metric.MetricPackage{
			MetricKey:   &key,
			MetricValue: &value,
		})
	}
	o.log.Debugf("Collected org quotas for %d orgs", len(metricsPackages))
	o.processedMetrics <- metricsPackages
}

func (o *OrgCollector) getTagsFromOrg(org cfclient.V3Organization) []string {
	tags := []string{}
	tags = append(tags, fmt.Sprintf("guid:%s", org.GUID))
	tags = append(tags, fmt.Sprintf("org_name:%s", org.Name))
	tags = append(tags, fmt.Sprintf("org_id:%s", org.GUID))

	status := "active"
	if org.Suspended != nil && *org.Suspended {
		status = "suspended"
	}

	tags = append(tags, fmt.Sprintf("status:%s", status))

	for k, v := range org.Metadata.Labels {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}

	for k, v := range org.Metadata.Annotations {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}

	return tags
}

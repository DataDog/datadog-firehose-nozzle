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
	cfClient, err := cloudfoundry.NewClient(config, log)
	if err != nil {
		return nil, err
	}
	return &OrgCollector{
		cfClient:         cfClient,
		log:              log,
		processedMetrics: processedMetrics,
		customTags:       customTags,
		queryInterval:    config.OrgDataQuerySeconds,
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
	ticker := time.NewTicker(time.Duration(o.queryInterval) * time.Second)
	for {
		select {
		case <-ticker.C:
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
	var allOrgs []cfclient.Org
	go func() {
		defer wg.Done()
		var err error
		allOrgs, err = o.cfClient.GetV2Orgs()
		if err != nil {
			errors <- err
		}
	}()

	// Fetch quotas
	wg.Add(1)
	var allQuotas []cfclient.OrgQuota
	go func() {
		defer wg.Done()
		var err error
		allQuotas, err = o.cfClient.GetV2OrgQuotas()
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
	quotaGuidsToObjects := map[string]cfclient.OrgQuota{}
	for _, q := range allQuotas {
		quotaGuidsToObjects[q.Guid] = q
	}

	metricsPackages := []metric.MetricPackage{}
	for _, org := range allOrgs {
		q, ok := quotaGuidsToObjects[org.QuotaDefinitionGuid]
		if !ok {
			o.log.Warnf("failed to get quota for org %s", org.Guid)
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

func (o *OrgCollector) getTagsFromOrg(org cfclient.Org) []string {
	tags := []string{}
	tags = append(tags, fmt.Sprintf("guid:%s", org.Guid))
	tags = append(tags, fmt.Sprintf("org_name:%s", org.Name))
	tags = append(tags, fmt.Sprintf("org_id:%s", org.Guid))
	tags = append(tags, fmt.Sprintf("status:%s", org.Status))
	return tags
}

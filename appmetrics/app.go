package appmetrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/sonde-go/events"
)

type App struct {
	Name                   string
	Host                   string
	Buildpack              string
	Command                string
	Diego                  bool
	OrgName                string
	OrgID                  string
	Routes                 []string
	SpaceID                string
	SpaceName              string
	SpaceURL               string
	GUID                   string
	DockerImage            string
	Instances              map[string]Instance
	NumberOfInstances      int
	TotalDiskConfigured    int
	TotalMemoryConfigured  int
	TotalDiskProvisioned   int
	TotalMemoryProvisioned int
	ErrorGrabbing          bool
	Tags                   []string
	updated                int64
	lock                   sync.RWMutex
}

type Instance struct {
	CellIP        string
	State         string
	InstanceIndex string
}

func newApp(guid string) *App {
	return &App{
		GUID:    guid,
		updated: time.Now().Unix(),
	}
}

func (a *App) getMetrics(customTags []string) []metrics.MetricPackage {
	var names = []string{
		"app.disk.configured",
		"app.disk.provisioned",
		"app.memory.configured",
		"app.memory.provisioned",
		"app.instances",
	}

	var ms = []float64{
		float64(a.TotalDiskConfigured),
		float64(a.TotalDiskProvisioned),
		float64(a.TotalMemoryConfigured),
		float64(a.TotalMemoryProvisioned),
		float64(a.NumberOfInstances),
	}

	return a.mkMetrics(names, ms, customTags)
}

func (a *App) parseContainerMetric(message *events.ContainerMetric, customTags []string) ([]metrics.MetricPackage, error) {
	var names = []string{
		"app.cpu.pct",
		"app.disk.used",
		"app.disk.quota",
		"app.memory.used",
		"app.memory.quota",
	}
	var ms = []float64{
		float64(message.GetCpuPercentage()),
		float64(message.GetDiskBytes()),
		float64(message.GetDiskBytesQuota()),
		float64(message.GetMemoryBytes()),
		float64(message.GetMemoryBytesQuota()),
	}
	tags := []string{fmt.Sprintf("instance:%v", message.GetInstanceIndex())}
	tags = append(tags, customTags...)

	return a.mkMetrics(names, ms, tags), nil
}

func (a *App) mkMetrics(names []string, ms []float64, moreTags []string) []metrics.MetricPackage {
	metricsPackages := []metrics.MetricPackage{}
	var host string
	if a.Host != "" {
		host = a.Host
	} else {
		host = a.GUID
	}

	tags := a.getTags()
	tags = append(tags, moreTags...)

	for i, name := range names {
		key := metrics.MetricKey{
			Name:     name,
			TagsHash: utils.HashTags(tags),
		}
		mVal := metrics.MetricValue{
			Tags: tags,
			Host: host,
		}
		p := metrics.Point{
			Timestamp: time.Now().Unix(),
			Value:     float64(ms[i]),
		}
		mVal.Points = append(mVal.Points, p)
		metricsPackages = append(metricsPackages, metrics.MetricPackage{
			MetricKey:   &key,
			MetricValue: &mVal,
		})
	}

	return metricsPackages
}

func (a *App) getTags() []string {
	if a.Tags != nil && len(a.Tags) > 0 {
		return a.Tags
	}

	a.Tags = a.generateTags()
	return a.Tags
}

func (a *App) generateTags() []string {
	var tags = []string{}
	if a.Name != "" {
		tags = append(tags, fmt.Sprintf("app_name:%v", a.Name))
	}
	if a.Buildpack != "" {
		tags = append(tags, fmt.Sprintf("buildpack:%v", a.Buildpack))
	}
	if a.Command != "" {
		tags = append(tags, fmt.Sprintf("command:%v", a.Command))
	}
	if a.Diego {
		tags = append(tags, fmt.Sprintf("diego"))
	}
	if a.OrgName != "" {
		tags = append(tags, fmt.Sprintf("org_name:%v", a.OrgName))
	}
	if a.OrgID != "" {
		tags = append(tags, fmt.Sprintf("org_id:%v", a.OrgID))
	}
	if a.SpaceName != "" {
		tags = append(tags, fmt.Sprintf("space_name:%v", a.SpaceName))
	}
	if a.SpaceID != "" {
		tags = append(tags, fmt.Sprintf("space_id:%v", a.SpaceID))
	}
	if a.SpaceURL != "" {
		tags = append(tags, fmt.Sprintf("space_url:%v", a.SpaceURL))
	}
	if a.GUID != "" {
		tags = append(tags, fmt.Sprintf("guid:%v", a.GUID))
	}
	if a.DockerImage != "" {
		tags = append(tags, fmt.Sprintf("image:%v", a.DockerImage))
	}

	return tags
}

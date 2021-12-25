package cloudfoundry

import "github.com/cloudfoundry-community/go-cfclient"

func (a *CFApplication) extractDataFromV3App(data cfclient.V3App) {
	a.GUID = data.GUID
	a.Name = data.Name
	a.SpaceGUID = data.Relationships["space"].Data.GUID
	a.Buildpacks = data.Lifecycle.BuildpackData.Buildpacks
	a.Annotations = data.Metadata.Annotations
	a.Labels = data.Metadata.Labels
	if a.Annotations == nil {
		a.Annotations = map[string]string{}
	}
	if a.Labels == nil {
		a.Labels = map[string]string{}
	}
}

func (a *CFApplication) extractDataFromV3Process(data []cfclient.Process) {
	if len(data) <= 0 {
		return
	}
	totalInstances := 0
	totalDiskInMbConfigured := 0
	totalDiskInMbProvisioned := 0
	totalMemoryInMbConfigured := 0
	totalMemoryInMbProvisioned := 0

	for _, p := range data {
		instances := p.Instances
		diskInMbConfigured := p.DiskInMB
		diskInMbProvisioned := instances * diskInMbConfigured
		memoryInMbConfigured := p.MemoryInMB
		memoryInMbProvisioned := instances * memoryInMbConfigured

		totalInstances += instances
		totalDiskInMbConfigured += diskInMbConfigured
		totalDiskInMbProvisioned += diskInMbProvisioned
		totalMemoryInMbConfigured += memoryInMbConfigured
		totalMemoryInMbProvisioned += memoryInMbProvisioned
	}

	a.Instances = totalInstances

	a.DiskQuota = totalDiskInMbConfigured
	a.Memory = totalMemoryInMbConfigured
	a.TotalDiskQuota = totalDiskInMbProvisioned
	a.TotalMemory = totalMemoryInMbProvisioned
}

func (a *CFApplication) extractDataFromV3Space(data cfclient.V3Space) {
	a.SpaceName = data.Name
	a.OrgGUID = data.Relationships["organization"].Data.GUID

	// Set space labels and annotations only if they're not overriden per application
	for key, value := range data.Metadata.Annotations {
		if _, ok := a.Annotations[key]; !ok {
			a.Annotations[key] = value
		}
	}
	for key, value := range data.Metadata.Labels {
		if _, ok := a.Labels[key]; !ok {
			a.Labels[key] = value
		}
	}
}

func (a *CFApplication) extractDataFromV3Org(data cfclient.V3Organization) {
	a.OrgName = data.Name

	// Set org labels and annotations only if they're not overriden per space or application
	for key, value := range data.Metadata.Annotations {
		if _, ok := a.Annotations[key]; !ok {
			a.Annotations[key] = value
		}
	}
	for key, value := range data.Metadata.Labels {
		if _, ok := a.Labels[key]; !ok {
			a.Labels[key] = value
		}
	}
}

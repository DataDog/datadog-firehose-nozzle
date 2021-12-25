package clusteragent

import "github.com/cloudfoundry-community/go-cfclient"

type CFApplication struct {
	GUID           string
	Name           string
	SpaceGUID      string
	SpaceName      string
	OrgName        string
	OrgGUID        string
	Instances      int
	Buildpacks     []string
	DiskQuota      int
	TotalDiskQuota int
	Memory         int
	TotalMemory    int
	Labels         map[string]string
	Annotations    map[string]string
}

func (a *CFApplication) setV3AppData(data cfclient.V3App) {
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

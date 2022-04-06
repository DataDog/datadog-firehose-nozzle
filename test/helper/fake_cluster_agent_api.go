package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// FakeClusterAgentAPI mocks a cloud controller
type FakeClusterAgentAPI struct {
	ReceivedContents chan []byte
	ReceivedRequests chan *http.Request
	usedEndpoints    []string

	server *httptest.Server
	lock   sync.Mutex

	validToken string

	tokenType   string
	accessToken string

	requested         bool
	lastAuthorization string

	closeMessage []byte

	// Used to make the controller slow to answer
	RequestTime time.Duration
}

// NewFakeClusterAgentAPI create a new cloud controller
func NewFakeClusterAgentAPI(tokenType string, accessToken string) *FakeClusterAgentAPI {
	return &FakeClusterAgentAPI{
		ReceivedContents: make(chan []byte, 100),
		ReceivedRequests: make(chan *http.Request, 100),
		usedEndpoints:    []string{},
		tokenType:        tokenType,
		accessToken:      accessToken,
		RequestTime:      0,
	}
}

// Start starts the cloud controller
func (f *FakeClusterAgentAPI) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

// Close closes the cloud controller
func (f *FakeClusterAgentAPI) Close() {
	f.server.Close()
}

// URL returns the API url
func (f *FakeClusterAgentAPI) URL() string {
	return f.server.URL
}

// ServeHTTP listens for http requests
func (f *FakeClusterAgentAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	contents, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	f.ReceivedContents <- contents
	f.ReceivedRequests <- r
	f.lock.Lock()
	f.usedEndpoints = append(f.usedEndpoints, r.URL.Path)
	f.lock.Unlock()

	time.Sleep(f.RequestTime * time.Millisecond)
	rw.Header().Set("Content-Type", "application/json")
	f.writeResponse(rw, r)

	f.lock.Lock()
	f.requested = true
	f.lock.Unlock()
}

func (f *FakeClusterAgentAPI) GetUsedEndpoints() []string {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.usedEndpoints
}

// AuthToken returns auth token
func (f *FakeClusterAgentAPI) AuthToken() string {
	if f.tokenType == "" && f.accessToken == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", f.tokenType, f.accessToken)
}

func (f *FakeClusterAgentAPI) writeResponse(rw http.ResponseWriter, r *http.Request) {
	// NOTE: app with GUID "6d254438-cc3b-44a6-b2e6-343ca92deb5f" is checked explicitly
	// in client_test.go, so any changes to this app and objects related to it will likely
	// result in failure of some of the tests in that file
	switch r.URL.Path {
	case "/version":
		rw.Write([]byte(`
		{
			"Major": 1,
			"Minor": 9,
			"Patch": 5,
			"Pre": "rc100",
			"Meta": "git.2598.2b50b49",
			"Commit": "2b50b49"
		}
		`))
	case "/api/v1/cf/apps":
		rw.Write([]byte(`
		[{
			"GUID": "a7bebd67-1991-4e9e-8d44-399acf2f13e8",
			"Name": "logs-backend-demo",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog_application_monitoring", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"app-space-org-label": "app-space-org-label-app-value",
				"app-space-label": "app-space-label-app-value",
				"app-org-label": "app-org-label-app-value",
				"app-label": "app-label-value",
				"space-org-label": "space-org-label-space-value",
				"space-label": "space-label-value",
				"org-label": "org-label-value"
			},
			"Annotations": {
				"app-space-org-annotation": "app-space-org-annotation-app-value",
				"app-space-annotation": "app-space-annotation-app-value",
				"app-org-annotation": "app-org-annotation-app-value",
				"app-annotation": "app-annotation-value",
				"space-org-annotation": "space-org-annotation-space-value",
				"space-annotation": "space-annotation-value",
				"org-annotation": "org-annotation-value"
			},
			"Sidecars": [
				{
					"Name": "sidecar-name-1",
					"GUID": "sidecar-guid-1"
				}
			]
		}, {
			"GUID": "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
			"Name": "hello-datadog-cf-ruby-dev",
			"SpaceGUID": "827da8e5-1676-42ec-9028-46fbfe04fb86",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 1,
			"Buildpacks": ["binary_buildpack", "datadog-cloudfoundry-buildpack-dev", "ruby_buildpack"],
			"DiskQuota": 2000,
			"TotalDiskQuota": 2048,
			"Memory": 1000,
			"TotalMemory": 512,
			"Labels": {
				"app-label": "app-label-value",
				"app-space-label": "app-space-label-app-value",
				"app-org-label": "app-org-label-app-value",
				"app-space-org-label": "app-space-org-label-app-value",
				"blacklisted_key": "bar",
				"tags.datadoghq.com/auto-label-tag": "auto-label-tag-value"
			},
			"Annotations": {
				"app-annotation": "app-annotation-value",
				"app-space-annotation": "app-space-annotation-app-value",
				"app-org-annotation": "app-org-annotation-app-value",
				"app-space-org-annotation": "app-space-org-annotation-app-value",
				"blacklisted_key": "bar",
				"tags.datadoghq.com/auto-annotation-tag": "auto-annotation-tag-value"
			},
			"Sidecars": null
		}, {
			"GUID": "9d519c2b-261e-4553-9db3-c79c8a9857f5",
			"Name": "test_log_redirect",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["https://github.com/DataDog/datadog-cloudfoundry-buildpack/releases/latest/download/datadog-cloudfoundry-buildpack.zip", "php_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "8987b9cc-3572-4afd-b17f-216be05a428a",
			"Name": "spring-sample",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["https://github.com/DataDog/datadog-cloudfoundry-buildpack/releases/download/4.20.0/datadog-cloudfoundry-buildpack-4.20.0.zip", "https://github.com/cloudfoundry/java-buildpack/releases/download/v4.40/java-buildpack-v4.40.zip"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "5957b5b7-4a29-4bd6-b1b2-5215b7c572f4",
			"Name": "test-rc3",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "integrations-lab-test-rc3",
				"tags.datadoghq.com/service": "autodiscovery-http-test-rc3"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "3.0.0-rc.3"
			},
			"Sidecars": null
		}, {
			"GUID": "9f43dcdd-9322-4555-ba67-02a9439fd925",
			"Name": "hello-datadog-cf-ruby-blue",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "0836312f-3063-46a8-a5bb-c50388b27ac9",
			"Name": "hello-datadog-cf-ruby-pcf-test",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "4bdf4890-1237-4f3f-a564-5bdda65e80c9",
			"Name": "test-apm-service-3",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "non-prod",
				"tags.datadoghq.com/service": "sinatra"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "2.0.0"
			},
			"Sidecars": null
		}, {
			"GUID": "06ed9382-26b8-42f3-9790-b0bf9f7c7fd9",
			"Name": "test-apm-service-4",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "non-prod-auto",
				"tags.datadoghq.com/service": "sinatra-auto"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "2.0.0"
			},
			"Sidecars": null
		}, {
			"GUID": "41256269-671b-4d79-91d5-3bf848376425",
			"Name": "test-space-org-cc",
			"SpaceGUID": "e8c645fb-c237-462e-945c-910bc8835b3b",
			"SpaceName": "datadog-application-monitoring-space",
			"OrgName": "datadog-application-monitoring-org",
			"OrgGUID": "955856da-6c1e-4a1a-9933-359bc0685855",
			"Instances": 2,
			"Buildpacks": ["datadog_application_monitoring", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "ead4c7fd-f21c-48b8-9f23-421f15a57cfc",
			"Name": "hello-datadog-cf-ruby-yellow",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"customTags": "noueman"
			},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "c7186718-9fb1-4a0a-9818-e2b29781ed60",
			"Name": "test-stdout",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "8035850f-ee2b-4f22-b725-69bbeb7d40f1",
			"Name": "hello-cf-datadog-scala",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": [],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "e6186b8d-1fa5-4907-89e3-bb035298cdc9",
			"Name": "hello-datadog-cf-ruby-pink",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "1f283863-86b2-47ba-8700-c4d71c6edea9",
			"Name": "test-apm-service-5",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "non-prod-auto-81",
				"tags.datadoghq.com/service": "sinatra-auto-81"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "2.0.0"
			},
			"Sidecars": null
		}, {
			"GUID": "dc22d0b9-d6a2-4389-84a6-76029121ebe4",
			"Name": "autodiscovery-http",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog_application_monitoring", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "integrations-lab",
				"tags.datadoghq.com/service": "autodiscovery-http"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "1.0.0"
			},
			"Sidecars": null
		}, {
			"GUID": "df33eca4-deb9-4cc0-b07e-4db8af3c7223",
			"Name": "hello-datadog-cf-ruby-pcf",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "14aa46da-2357-40d9-988d-2c6f3660680d",
			"Name": "hello-datadog-cf-ruby-test",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog-cloudfoundry-buildpack", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "47465e0b-afe6-4ab4-8262-a56ad88bf6dc",
			"Name": "hello-datadog-cf-scala",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "java_buildpack_offline"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {},
			"Annotations": {},
			"Sidecars": null
		}, {
			"GUID": "8957b9a0-4132-4754-acc5-e3b959b5c77a",
			"Name": "test-apm-service",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "non-prod",
				"tags.datadoghq.com/service": "sinatra"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "2.0.0"
			},
			"Sidecars": null
		}, {
			"GUID": "0ba38e35-614c-4576-862a-92a1b60c53db",
			"Name": "test-apm-service-2",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["dd-cf-bp", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"tags.datadoghq.com/env": "non-prod",
				"tags.datadoghq.com/service": "sinatra"
			},
			"Annotations": {
				"tags.datadoghq.com/version": "2.0.0"
			}
		}]
		`))
	case "/api/v1/cf/apps/a7bebd67-1991-4e9e-8d44-399acf2f13e8":
		rw.Write([]byte(`
		{
			"GUID": "a7bebd67-1991-4e9e-8d44-399acf2f13e8",
			"Name": "logs-backend-demo",
			"SpaceGUID": "68a6159c-cb4a-4f70-bb48-f2c24bd79c6b",
			"SpaceName": "system",
			"OrgName": "system",
			"OrgGUID": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"Instances": 2,
			"Buildpacks": ["datadog_application_monitoring", "ruby_buildpack"],
			"DiskQuota": 1024,
			"TotalDiskQuota": 2048,
			"Memory": 256,
			"TotalMemory": 512,
			"Labels": {
				"app-space-org-label": "app-space-org-label-app-value",
				"app-space-label": "app-space-label-app-value",
				"app-org-label": "app-org-label-app-value",
				"app-label": "app-label-value",
				"space-org-label": "space-org-label-space-value",
				"space-label": "space-label-value",
				"org-label": "org-label-value"
			},
			"Annotations": {
				"app-space-org-annotation": "app-space-org-annotation-app-value",
				"app-space-annotation": "app-space-annotation-app-value",
				"app-org-annotation": "app-org-annotation-app-value",
				"app-annotation": "app-annotation-value",
				"space-org-annotation": "space-org-annotation-space-value",
				"space-annotation": "space-annotation-value",
				"org-annotation": "org-annotation-value"
			},
			"Sidecars": [
				{
					"Name": "sidecar-name-1",
					"GUID": "sidecar-guid-1"
				}
			]
		}
		`))
	case "/api/v1/cf/org_quotas":
		rw.Write([]byte(`
		[{
			"GUID": "9f3f17e5-dabe-49b9-82c6-aa9e79724bdd",
			"MemoryLimit": 10240
		}, {
			"GUID": "c43e81cd-c305-4fa1-9cad-0f7bd72ab6c4",
			"MemoryLimit": 102400
		}]
		`))
	case "/api/v1/cf/orgs":
		rw.Write([]byte(`
		[{
			"name": "system",
			"guid": "24d7098c-832b-4dfa-a4f1-950780ae92e9",
			"suspended": false,
			"created_at": "2021-03-17T10:39:43Z",
			"updated_at": "2021-03-17T10:39:43Z",
			"relationships": {
				"quota": {
					"data": {
						"guid": "9f3f17e5-dabe-49b9-82c6-aa9e79724bdd"
					}
				}
			},
			"links": {
				"default_domain": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/24d7098c-832b-4dfa-a4f1-950780ae92e9/domains/default"
				},
				"domains": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/24d7098c-832b-4dfa-a4f1-950780ae92e9/domains"
				},
				"quota": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organization_quotas/9f3f17e5-dabe-49b9-82c6-aa9e79724bdd"
				},
				"self": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/24d7098c-832b-4dfa-a4f1-950780ae92e9"
				}
			},
			"metadata": {
				"annotations": {
					"org-annotation": "org-annotation-value",
					"app-org-annotation": "app-org-annotation-org-value",
					"space-org-annotation": "space-org-annotation-org-value",
					"app-space-org-annotation": "app-space-org-annotation-org-value"
				},
				"labels": {
					"org-label": "org-label-value",
					"app-org-label": "app-org-label-org-value",
					"space-org-label": "space-org-label-org-value",
					"app-space-org-label": "app-space-org-label-org-value"
				}
			}
		}, {
			"name": "datadog-application-monitoring-org",
			"guid": "955856da-6c1e-4a1a-9933-359bc0685855",
			"suspended": false,
			"created_at": "2021-03-31T16:26:52Z",
			"updated_at": "2021-03-31T16:26:53Z",
			"relationships": {
				"quota": {
					"data": {
						"guid": "c43e81cd-c305-4fa1-9cad-0f7bd72ab6c4"
					}
				}
			},
			"links": {
				"default_domain": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/955856da-6c1e-4a1a-9933-359bc0685855/domains/default"
				},
				"domains": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/955856da-6c1e-4a1a-9933-359bc0685855/domains"
				},
				"quota": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organization_quotas/c43e81cd-c305-4fa1-9cad-0f7bd72ab6c4"
				},
				"self": {
					"href": "https://api.sys.integrations-lab.devenv.dog/v3/organizations/955856da-6c1e-4a1a-9933-359bc0685855"
				}
			},
			"metadata": {
				"annotations": {
					"org-annotation": "org-annotation-value",
					"app-org-annotation": "app-org-annotation-org-value",
					"space-org-annotation": "space-org-annotation-org-value",
					"app-space-org-annotation": "app-space-org-annotation-org-value"
				},
				"labels": {
					"org-label": "org-label-value",
					"app-org-label": "app-org-label-org-value",
					"space-org-label": "space-org-label-org-value",
					"app-space-org-label": "app-space-org-label-org-value"
				}
			}
		}]
		`))
	}
}

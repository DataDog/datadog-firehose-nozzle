package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"github.com/cloudfoundry/sonde-go/events"
)

// FakeCloudControllerAPI mocks a cloud controller
type FakeCloudControllerAPI struct {
	ReceivedContents chan []byte
	ReceivedRequests chan *http.Request

	server *httptest.Server
	lock   sync.Mutex

	validToken string

	tokenType   string
	accessToken string

	lastAuthorization string
	requested         bool

	events       []events.Envelope
	closeMessage []byte
}

// NewFakeCloudControllerAPI create a new cloud controller
func NewFakeCloudControllerAPI(tokenType string, accessToken string) *FakeCloudControllerAPI {
	return &FakeCloudControllerAPI{
		ReceivedContents: make(chan []byte, 100),
		ReceivedRequests: make(chan *http.Request, 100),
		tokenType:        tokenType,
		accessToken:      accessToken,
	}
}

// Start starts the cloud controller
func (f *FakeCloudControllerAPI) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

// Close closes the cloud controller
func (f *FakeCloudControllerAPI) Close() {
	f.server.Close()
}

// URL returns the API url
func (f *FakeCloudControllerAPI) URL() string {
	return f.server.URL
}

// ServeHTTP listens for http requests
func (f *FakeCloudControllerAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	contents, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	go func() {
		f.ReceivedContents <- contents
		f.ReceivedRequests <- r
	}()

	f.writeResponse(rw, r)

	f.lock.Lock()
	f.requested = true
	f.lock.Unlock()
}

// AuthToken returns auth token
func (f *FakeCloudControllerAPI) AuthToken() string {
	if f.tokenType == "" && f.accessToken == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", f.tokenType, f.accessToken)
}

func (f *FakeCloudControllerAPI) writeResponse(rw http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v2/info":
		rw.Write([]byte(fmt.Sprintf(`
		{
			"name": "Small Footprint PAS",
			"build": "2.4.7-build.16",
			"support": "https://support.pivotal.io",
			"version": 0,
			"description": "https://docs.pivotal.io/pivotalcf/2-3/pcf-release-notes/runtime-rn.html",
			"authorization_endpoint": "%s",
			"token_endpoint": "%s",
			"min_cli_version": "6.23.0",
			"min_recommended_cli_version": "6.23.0",
			"api_version": "2.125.0",
			"osbapi_version": "2.14",
			"routing_endpoint": "%s/routing"
		  }
		`, f.URL(), f.URL(), f.URL())))
	case "/v2/apps":
		params, _ := url.ParseQuery(r.URL.RawQuery)
		page := params["page"][0]
		rw.Write([]byte(fmt.Sprintf(`
		{
			"total_results": 4,
			"total_pages": 4,
			"prev_url": null,
			"next_url": "/v2/apps?inline-relations-depth=2&order-direction=asc&page=2&results-per-page=1",
			"resources": [
			  {
				"metadata": {
				  "guid": "app-%s",
				  "url": "/v2/apps/b7f0436c-c1c7-499f-b9af-33e593a3f0ea",
				  "created_at": "2019-05-17T15:02:39Z",
				  "updated_at": "2019-05-21T13:51:14Z"
				},
				"entity": {
				  "name": "app-%s",
				  "production": false,
				  "space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				  "stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				  "buildpack": "ruby_buildpack",
				  "detected_buildpack": "ruby",
				  "detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				  "environment_json": {},
				  "memory": 1024,
				  "instances": 1,
				  "disk_quota": 1024,
				  "state": "STOPPED",
				  "version": "7ab56b40-5b6f-41b3-a731-33d5228dc94f",
				  "command": "bundle exec rake scheduler:start",
				  "console": false,
				  "debug": null,
				  "staging_task_id": "212edfb9-85d2-4f18-8a1a-901539358167",
				  "package_state": "STAGED",
				  "health_check_type": "process",
				  "health_check_timeout": null,
				  "health_check_http_endpoint": "",
				  "staging_failed_reason": null,
				  "staging_failed_description": null,
				  "diego": true,
				  "docker_image": null,
				  "docker_credentials": {
					"username": null,
					"password": null
				  },
				  "package_updated_at": "2019-05-17T15:02:41Z",
				  "detected_start_command": "bin/rails server -b 0.0.0.0 -p $PORT -e $RAILS_ENV",
				  "enable_ssh": true,
				  "ports": [
					8080
				  ],
				  "space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				  "space": {
					"metadata": {
					  "guid": "417b893e-291e-48ec-94c7-7b2348604365",
					  "url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
					  "created_at": "2019-05-17T15:02:37Z",
					  "updated_at": "2019-05-17T15:02:37Z"
					},
					"entity": {
					  "name": "system",
					  "organization_guid": "671557cf-edcd-49df-9863-ee14513d13c7",
					  "space_quota_definition_guid": null,
					  "isolation_segment_guid": null,
					  "allow_ssh": true,
					  "organization_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7",
					  "organization": {
						"metadata": {
						  "guid": "671557cf-edcd-49df-9863-ee14513d13c7",
						  "url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7",
						  "created_at": "2019-05-17T13:06:27Z",
						  "updated_at": "2019-05-21T13:52:13Z"
						},
						"entity": {
						  "name": "system",
						  "billing_enabled": false,
						  "quota_definition_guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e",
						  "status": "active",
						  "default_isolation_segment_guid": null,
						  "quota_definition_url": "/v2/quota_definitions/1cf98856-aba8-49a8-8b21-d82a25898c4e",
						  "spaces_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/spaces",
						  "domains_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/domains",
						  "private_domains_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/private_domains",
						  "users_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/users",
						  "managers_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/managers",
						  "billing_managers_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/billing_managers",
						  "auditors_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/auditors",
						  "app_events_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/app_events",
						  "space_quota_definitions_url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7/space_quota_definitions"
						}
					  },
					  "developers_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/developers",
					  "developers": [],
					  "managers_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/managers",
					  "managers": [],
					  "auditors_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/auditors",
					  "auditors": [],
					  "apps_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/apps",
					  "routes_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/routes",
					  "domains_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/domains",
					  "domains": [],
					  "service_instances_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/service_instances",
					  "service_instances": [],
					  "app_events_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/app_events",
					  "events_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/events",
					  "security_groups_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/security_groups",
					  "security_groups": [],
					  "staging_security_groups_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365/staging_security_groups",
					  "staging_security_groups": []
					}
				  },
				  "stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				  "stack": {
					"metadata": {
					  "guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
					  "url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
					  "created_at": "2019-05-17T13:06:27Z",
					  "updated_at": "2019-05-17T13:06:27Z"
					},
					"entity": {
					  "name": "cflinuxfs3",
					  "description": "Cloud Foundry Linux-based filesystem - Ubuntu Bionic 18.04 LTS"
					}
				  },
				  "routes_url": "/v2/apps/b7f0436c-c1c7-499f-b9af-33e593a3f0ea/routes",
				  "routes": [],
				  "events_url": "/v2/apps/b7f0436c-c1c7-499f-b9af-33e593a3f0ea/events",
				  "service_bindings_url": "/v2/apps/b7f0436c-c1c7-499f-b9af-33e593a3f0ea/service_bindings",
				  "service_bindings": [],
				  "route_mappings_url": "/v2/apps/b7f0436c-c1c7-499f-b9af-33e593a3f0ea/route_mappings"
				}
			  }
			]
		  }`, page, page)))
	default:
		rw.Write([]byte(fmt.Sprintf(`
		{
			"token_type": "%s",
			"access_token": "%s"
		}
		`, f.tokenType, f.accessToken)))
	}
}

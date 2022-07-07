package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// FakeCloudControllerAPI mocks a cloud controller
type FakeCloudControllerAPI struct {
	ReceivedContents chan []byte
	ReceivedRequests chan *http.Request
	usedEndpoints    []string

	server *httptest.Server
	lock   sync.Mutex

	validToken string

	tokenType   string
	accessToken string

	lastAuthorization string
	requested         bool

	closeMessage []byte

	// Used to make the controller slow to answer
	RequestTime time.Duration

	// Number of apps
	AppNumber int
}

// NewFakeCloudControllerAPI create a new cloud controller
func NewFakeCloudControllerAPI(tokenType string, accessToken string) *FakeCloudControllerAPI {
	return &FakeCloudControllerAPI{
		ReceivedContents: make(chan []byte, 100),
		ReceivedRequests: make(chan *http.Request, 100),
		usedEndpoints:    []string{},
		tokenType:        tokenType,
		accessToken:      accessToken,
		RequestTime:      0,
		AppNumber:        4,
	}
}

// Start starts the cloud controller
func (f *FakeCloudControllerAPI) Start(wg *sync.WaitGroup) {
	f.server = httptest.NewUnstartedServer(f)
	wg.Done()
	f.server.Start()
}

// Close closes the cloud controller
func (f *FakeCloudControllerAPI) Close() {
	f.server.CloseClientConnections()
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

func (f *FakeCloudControllerAPI) GetUsedEndpoints() []string {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.usedEndpoints
}

// AuthToken returns auth token
func (f *FakeCloudControllerAPI) AuthToken() string {
	if f.tokenType == "" && f.accessToken == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", f.tokenType, f.accessToken)
}

func (f *FakeCloudControllerAPI) writeResponse(rw http.ResponseWriter, r *http.Request) {
	// NOTE: app with GUID "6d254438-cc3b-44a6-b2e6-343ca92deb5f" is checked explicitly
	// in client_test.go, so any changes to this app and objects related to it will likely
	// result in failure of some of the tests in that file
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
		rw.Write([]byte(fmt.Sprintf(`
		{
		  "total_results": 15,
		  "total_pages": 3,
		  "prev_url": null,
		  "next_url": null,
		  "resources": [
			{
			  "metadata": {
					"guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f",
					"url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f",
					"created_at": "2019-05-17T15:06:02Z",
					"updated_at": "2019-10-04T11:11:00Z"
				},
				"entity": {
					"name": "p-invitations-green",
					"production": false,
					"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
					"space": {
						"metadata": {
							"guid": "417b893e-291e-48ec-94c7-7b2348604365"
						},
						"entity": {
							"name": "system",
							"organization": {
								"metadata": {
									"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
								},
								"entity": {
									"name": "system"
								}
							}
						}
					},
					"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
					"buildpack": "nodejs_buildpack",
					"detected_buildpack": "nodejs",
					"detected_buildpack_guid": "26c169f4-0e24-49a6-a60c-b16ce5eea318",
					"environment_json": {
						"CLOUD_CONTROLLER_URL": "https://cloudfoundry.env",
						"COMPANY_NAME": "Pivotal",
						"INVITATIONS_CLIENT_ID": "invitations",
						"NODE_TLS_REJECT_UNAUTHORIZED": "0",
						"PRODUCT_NAME": "Apps Manager",
						"SUCCESS_CALLBACK_URL": "",
						"SUPPORT_EMAIL": ""
					},
					"memory": 256,
					"instances": 1,
					"disk_quota": 1024,
					"state": "STARTED",
					"version": "49479c78-27de-4398-ad37-b715f265ac4c",
					"command": null,
					"console": false,
					"debug": null,
					"staging_task_id": "ef7cfe60-d758-4a90-9113-4a2f852265b4",
					"package_state": "STAGED",
					"health_check_type": "port",
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
					"package_updated_at": "2019-10-04T11:10:26Z",
					"detected_start_command": "npm start",
					"enable_ssh": true,
					"ports": [
						8080
					],
					"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
					"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
					"routes_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/routes",
					"events_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/events",
					"service_bindings_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/service_bindings",
					"route_mappings_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/route_mappings"
				}
			},
			{
			  "metadata": {
				"guid": "487945cb-c486-4f7b-b313-139a0a686d31",
				"url": "/v2/apps/487945cb-c486-4f7b-b313-139a0a686d31",
				"created_at": "2019-05-17T15:12:21Z",
				"updated_at": "2019-10-04T11:19:06Z"
			  },
			  "entity": {
				"name": "nfsbroker",
				"production": false,
				"space_guid": "8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "binary",
				"detected_buildpack_guid": "7a9e537d-2692-4385-9594-dd5efed2c705",
				"environment_json": {
				  "DB_USERNAME": "fTXVlrjrCKYmmdLwwUlM",
				  "USERNAME": "isuIIWepWUlXPpQPkiQA"
				},
				"memory": 256,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "ba74ffd0-cede-4a25-a6dd-37a27120acba",
				"command": null,
				"console": false,
				"debug": null,
				"staging_task_id": "960a885e-7efb-42b9-9df1-472ee20e27d7",
				"package_state": "STAGED",
				"health_check_type": "port",
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
				"package_updated_at": "2019-10-04T11:19:00Z",
				"detected_start_command": "./start.sh",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/487945cb-c486-4f7b-b313-139a0a686d31/routes",
				"events_url": "/v2/apps/487945cb-c486-4f7b-b313-139a0a686d31/events",
				"service_bindings_url": "/v2/apps/487945cb-c486-4f7b-b313-139a0a686d31/service_bindings",
				"route_mappings_url": "/v2/apps/487945cb-c486-4f7b-b313-139a0a686d31/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "6f1fbbf4-b04c-4574-a7be-cb059e170287",
				"url": "/v2/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287",
				"created_at": "2019-05-17T15:23:05Z",
				"updated_at": "2019-05-23T15:50:16Z"
			  },
			  "entity": {
				"name": "gcp-service-broker-4.2.2",
				"production": false,
				"space_guid": "417ca75c-3fea-4ea2-8428-b02bdf05deb0",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "go_buildpack",
				"detected_buildpack": "go",
				"detected_buildpack_guid": "97b8d120-4fc5-4543-bc11-dafe1fac3e22",
				"environment_json": {
				  "VERIFY_SSL": "true"
				},
				"memory": 1024,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "bc1117e6-d51d-4f38-af87-48a82c589f65",
				"command": null,
				"console": false,
				"debug": null,
				"staging_task_id": "d07b0c9b-116f-419e-8428-7b13e2dbce0c",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": null,
				"health_check_http_endpoint": null,
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-05-23T15:49:22Z",
				"detected_start_command": "gcp-service-broker",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417ca75c-3fea-4ea2-8428-b02bdf05deb0",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/routes",
				"events_url": "/v2/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/events",
				"service_bindings_url": "/v2/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/service_bindings",
				"route_mappings_url": "/v2/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "8054a565-d476-4535-807c-57e311da5051",
				"url": "/v2/apps/8054a565-d476-4535-807c-57e311da5051",
				"created_at": "2019-05-21T12:15:09Z",
				"updated_at": "2019-09-03T08:51:08Z"
			  },
			  "entity": {
				"name": "hello-datadog-cf-ruby",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "RUN_AGENT": "true",
				  "STD_LOG_COLLECTION_PORT": "10514"
				},
				"memory": 100,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "439991a1-ca6c-4503-afe3-b429eab16568",
				"command": "ruby app.rb",
				"console": false,
				"debug": null,
				"staging_task_id": "a1b5baa2-c4fe-4c88-b189-5e57772afe47",
				"package_state": "STAGED",
				"health_check_type": "port",
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
				"package_updated_at": "2019-09-02T15:45:42Z",
				"detected_start_command": "ruby app.rb",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/8054a565-d476-4535-807c-57e311da5051/routes",
				"events_url": "/v2/apps/8054a565-d476-4535-807c-57e311da5051/events",
				"service_bindings_url": "/v2/apps/8054a565-d476-4535-807c-57e311da5051/service_bindings",
				"route_mappings_url": "/v2/apps/8054a565-d476-4535-807c-57e311da5051/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
				"url": "/v2/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
				"created_at": "2019-08-29T22:05:40Z",
				"updated_at": "2019-08-29T22:07:10Z"
			  },
			  "entity": {
				"name": "hello-datadog-cf-ruby-dev",
				"production": false,
				"space_guid": "827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": null,
				"detected_buildpack_guid": null,
				"environment_json": {
				  "RUN_AGENT": "true",
				  "STD_LOG_COLLECTION_PORT": "10514"
				},
				"memory": 1000,
				"instances": 1,
				"disk_quota": 2000,
				"state": "STOPPED",
				"version": "68e67fb6-46f8-4c83-8b5b-7c155d60e8cb",
				"command": "ruby app.rb",
				"console": false,
				"debug": null,
				"staging_task_id": "4d1056a2-65fb-4516-b8ea-f8b8577aab81",
				"package_state": "FAILED",
				"health_check_type": "port",
				"health_check_timeout": null,
				"health_check_http_endpoint": null,
				"staging_failed_reason": "InsufficientResources",
				"staging_failed_description": "Insufficient resources",
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-08-29T22:05:40Z",
				"detected_start_command": "",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/routes",
				"events_url": "/v2/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/events",
				"service_bindings_url": "/v2/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/service_bindings",
				"route_mappings_url": "/v2/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "771b41ca-d38f-4f4c-817d-80e5df4b11e0",
				"url": "/v2/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0",
				"created_at": "2019-08-30T21:59:08Z",
				"updated_at": "2019-09-18T17:03:33Z"
			  },
			  "entity": {
				"name": "hello-datadog-cf-ruby-dev-nick",
				"production": false,
				"space_guid": "827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "DD_DEBUG_STD_REDIRECTION": "true",
				  "DD_ENABLE_CHECKS": "true",
				  "DD_JMXFETCH_ENABLED": "true",
				  "DD_LOGS_CONFIG_USE_HTTP": "false",
				  "DD_LOGS_ENABLED": "true",
				  "DD_LOG_LEVEL": "Info",
				  "DD_SITE": "datadoghq.eu",
				  "DD_TAGS": "nick:buildpack-test-logs-endpoint-eu",
				  "RUN_AGENT": "true",
				  "STD_LOG_COLLECTION_PORT": "10514"
				},
				"memory": 100,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "09c36cbf-2c77-435c-becd-3c94439c1ecc",
				"command": "ruby app.rb",
				"console": false,
				"debug": null,
				"staging_task_id": "7ab6cf06-2235-4b40-a395-777370753b30",
				"package_state": "STAGED",
				"health_check_type": "port",
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
				"package_updated_at": "2019-09-18T17:03:29Z",
				"detected_start_command": "ruby app.rb",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/routes",
				"events_url": "/v2/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/events",
				"service_bindings_url": "/v2/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/service_bindings",
				"route_mappings_url": "/v2/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "b04d2676-ce47-4ad4-bf5c-0b4ffe134556",
				"url": "/v2/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556",
				"created_at": "2019-09-09T18:13:41Z",
				"updated_at": "2019-10-04T11:08:54Z"
			  },
			  "entity": {
				"name": "app-usage-worker-venerable",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "ruby_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "ALLOW_TEST_DATA_CREATION": "false",
				  "ALLOW_VIEWING_LOGS": "false",
				  "BUNDLE_WITHOUT": "test:development",
				  "EVENT_FETCHING_OFFSET": "60",
				  "RACK_ENV": "production",
				  "RAILS_ENV": "production",
				  "SKIP_SSL_VALIDATION": "true",
				  "USAGE_SERVICE_UAA_CLIENT_ID": "usage_service"
				},
				"memory": 2048,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STOPPED",
				"version": "516c81f4-5086-4a06-bfe9-260caeb20766",
				"command": "bundle exec rake worker:start",
				"console": false,
				"debug": null,
				"staging_task_id": "f0be5269-b708-4554-a6a6-a9fafd5bbf0c",
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
				"package_updated_at": "2019-09-09T18:13:42Z",
				"detected_start_command": "bin/rails server -b 0.0.0.0 -p $PORT -e $RAILS_ENV",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/routes",
				"events_url": "/v2/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/events",
				"service_bindings_url": "/v2/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/service_bindings",
				"route_mappings_url": "/v2/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "2697a7af-4190-402a-b3b7-de31c6065328",
				"url": "/v2/apps/2697a7af-4190-402a-b3b7-de31c6065328",
				"created_at": "2019-10-04T11:06:31Z",
				"updated_at": "2019-10-04T11:09:06Z"
			  },
			  "entity": {
				"name": "app-usage-scheduler",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "ruby_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "ALLOW_TEST_DATA_CREATION": "false",
				  "ALLOW_VIEWING_LOGS": "false",
				  "BUNDLE_WITHOUT": "test:development",
				  "USAGE_SERVICE_UAA_CLIENT_ID": "usage_service"
				},
				"memory": 1024,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "615e5f84-6a2c-428e-b359-f9aacf956c04",
				"command": "bundle exec rake scheduler:start",
				"console": false,
				"debug": null,
				"staging_task_id": "134fc768-fd1d-4902-a64b-476c8add9573",
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
				"package_updated_at": "2019-10-04T11:06:31Z",
				"detected_start_command": "bin/rails server -b 0.0.0.0 -p $PORT -e $RAILS_ENV",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/2697a7af-4190-402a-b3b7-de31c6065328/routes",
				"events_url": "/v2/apps/2697a7af-4190-402a-b3b7-de31c6065328/events",
				"service_bindings_url": "/v2/apps/2697a7af-4190-402a-b3b7-de31c6065328/service_bindings",
				"route_mappings_url": "/v2/apps/2697a7af-4190-402a-b3b7-de31c6065328/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "aa2431c4-9536-4f87-b78c-9d5a44717431",
				"url": "/v2/apps/aa2431c4-9536-4f87-b78c-9d5a44717431",
				"created_at": "2019-10-04T11:06:31Z",
				"updated_at": "2019-10-04T11:09:05Z"
			  },
			  "entity": {
				"name": "app-usage-worker",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "ruby_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "ALLOW_TEST_DATA_CREATION": "false",
				  "ALLOW_VIEWING_LOGS": "false",
				  "BUNDLE_WITHOUT": "test:development"
				},
				"memory": 2048,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "b0a338ee-70fe-441a-be56-7972c868ba32",
				"command": "bundle exec rake worker:start",
				"console": false,
				"debug": null,
				"staging_task_id": "9f9f0fcd-9c9f-47f7-930e-fde34e874c17",
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
				"package_updated_at": "2019-10-04T11:06:32Z",
				"detected_start_command": "bin/rails server -b 0.0.0.0 -p $PORT -e $RAILS_ENV",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/routes",
				"events_url": "/v2/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/events",
				"service_bindings_url": "/v2/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/service_bindings",
				"route_mappings_url": "/v2/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "c482ca96-ac53-455e-9e29-a5cff5ef176a",
				"url": "/v2/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a",
				"created_at": "2019-10-04T11:06:31Z",
				"updated_at": "2019-10-04T11:09:06Z"
			  },
			  "entity": {
				"name": "app-usage-server",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "ruby_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "ALLOW_TEST_DATA_CREATION": "false",
				  "ALLOW_VIEWING_LOGS": "false",
				  "BUNDLE_WITHOUT": "test:development"
				},
				"memory": 1024,
				"instances": 2,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "bf2cd9b9-3ac2-4024-80ac-595ba9121ae1",
				"command": "bundle exec rake server:start",
				"console": false,
				"debug": null,
				"staging_task_id": "b7b186c7-7999-4db3-8fc6-150058660cab",
				"package_state": "STAGED",
				"health_check_type": "http",
				"health_check_timeout": 180,
				"health_check_http_endpoint": "/heartbeat/db",
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-04T11:06:32Z",
				"detected_start_command": "bin/rails server -b 0.0.0.0 -p $PORT -e $RAILS_ENV",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/routes",
				"events_url": "/v2/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/events",
				"service_bindings_url": "/v2/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/service_bindings",
				"route_mappings_url": "/v2/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "74e907a6-690a-4310-8804-c0b3beb5d302",
				"url": "/v2/apps/74e907a6-690a-4310-8804-c0b3beb5d302",
				"created_at": "2019-10-04T11:10:26Z",
				"updated_at": "2019-10-04T11:10:50Z"
			  },
			  "entity": {
				"name": "apps-manager-js-green",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "staticfile_buildpack",
				"detected_buildpack": "staticfile",
				"detected_buildpack_guid": "fc4fdd64-9a6c-4409-9c7d-dc41ac145457",
				"environment_json": {
				  "ACCENT_COLOR": "#00A79D"
				},
				"memory": 128,
				"instances": 2,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "14dca8ab-04c9-4b94-80ff-d0748e65ac9e",
				"command": "$HOME/boot.sh",
				"console": false,
				"debug": null,
				"staging_task_id": "61fc897b-d959-4407-a9e4-62244501e55e",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": null,
				"health_check_http_endpoint": null,
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-04T11:10:26Z",
				"detected_start_command": "$HOME/boot.sh",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/74e907a6-690a-4310-8804-c0b3beb5d302/routes",
				"events_url": "/v2/apps/74e907a6-690a-4310-8804-c0b3beb5d302/events",
				"service_bindings_url": "/v2/apps/74e907a6-690a-4310-8804-c0b3beb5d302/service_bindings",
				"route_mappings_url": "/v2/apps/74e907a6-690a-4310-8804-c0b3beb5d302/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "46592861-ab1b-4088-ba13-9e09038d0054",
				"url": "/v2/apps/46592861-ab1b-4088-ba13-9e09038d0054",
				"created_at": "2019-10-04T11:12:26Z",
				"updated_at": "2019-10-04T11:12:49Z"
			  },
			  "entity": {
				"name": "notifications-ui",
				"production": false,
				"space_guid": "1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "binary",
				"detected_buildpack_guid": "7a9e537d-2692-4385-9594-dd5efed2c705",
				"environment_json": {
				  "VERIFY_SSL": "false"
				},
				"memory": 64,
				"instances": 2,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "7c39ad2b-b72b-4611-827f-7dc9ce9a25ab",
				"command": "bin/notifications-ui",
				"console": false,
				"debug": null,
				"staging_task_id": "3c13315e-96a4-4726-8e6f-dee271d4b5a6",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": null,
				"health_check_http_endpoint": null,
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-04T11:12:27Z",
				"detected_start_command": ">&2 echo Error: no start command specified during staging or launch && exit 1",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/46592861-ab1b-4088-ba13-9e09038d0054/routes",
				"events_url": "/v2/apps/46592861-ab1b-4088-ba13-9e09038d0054/events",
				"service_bindings_url": "/v2/apps/46592861-ab1b-4088-ba13-9e09038d0054/service_bindings",
				"route_mappings_url": "/v2/apps/46592861-ab1b-4088-ba13-9e09038d0054/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "b096b43f-3c4f-4b5d-94a6-556cfcca17c0",
				"url": "/v2/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0",
				"created_at": "2019-10-04T11:13:25Z",
				"updated_at": "2019-10-04T11:13:42Z"
			  },
			  "entity": {
				"name": "autoscale",
				"production": false,
				"space_guid": "d5d005a4-0320-4daa-ac0a-81f8dcd00fe0",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "binary",
				"detected_buildpack_guid": "7a9e537d-2692-4385-9594-dd5efed2c705",
				"environment_json": {
				  "API_CLIENT_ID": "autoscaling_api",
				  "VERIFY_SSL": "false"
				},
				"memory": 256,
				"instances": 3,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "3e522be2-1c1b-4ebc-830d-80efd964899c",
				"command": "bin/cf-autoscaling",
				"console": false,
				"debug": null,
				"staging_task_id": "ea2a9608-6d93-4b86-9156-b206ecb57b6e",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": null,
				"health_check_http_endpoint": null,
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-04T11:13:25Z",
				"detected_start_command": ">&2 echo Error: no start command specified during staging or launch && exit 1",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/routes",
				"events_url": "/v2/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/events",
				"service_bindings_url": "/v2/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/service_bindings",
				"route_mappings_url": "/v2/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04",
				"url": "/v2/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04",
				"created_at": "2019-10-04T11:13:43Z",
				"updated_at": "2019-10-04T11:15:10Z"
			  },
			  "entity": {
				"name": "autoscale-api",
				"production": false,
				"space_guid": "d5d005a4-0320-4daa-ac0a-81f8dcd00fe0",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "java_buildpack_offline",
				"detected_buildpack": "",
				"detected_buildpack_guid": "19ab64a0-7d90-4148-bc4d-70c0c9eb7d31",
				"environment_json": {
				  "SKIP_CERT_VERIFY": "true"
				},
				"memory": 1024,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "84990f8b-4df4-468e-b053-22a442aeba52",
				"command": null,
				"console": false,
				"debug": null,
				"staging_task_id": "9d850a6b-597c-42d1-9f59-7ddc6dc9a229",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": 120,
				"health_check_http_endpoint": null,
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-04T11:13:43Z",
				"detected_start_command": "org.springframework.boot.loader.JarLauncher",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/routes",
				"events_url": "/v2/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/events",
				"service_bindings_url": "/v2/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/service_bindings",
				"route_mappings_url": "/v2/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/route_mappings"
			  }
			},
			{
			  "metadata": {
				"guid": "7604d784-6ada-4b13-8a22-d892d8fa972d",
				"url": "/v2/apps/7604d784-6ada-4b13-8a22-d892d8fa972d",
				"created_at": "2019-10-08T20:01:39Z",
				"updated_at": "2019-10-08T21:11:28Z"
			  },
			  "entity": {
				"name": "hello-datadog-cf-ruby-dev-nick-no-python",
				"production": false,
				"space_guid": "827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "binary_buildpack",
				"detected_buildpack": "ruby",
				"detected_buildpack_guid": "3ddc6864-db9c-4dd4-8d15-ae690363ceb2",
				"environment_json": {
				  "DD_ENABLE_CHECKS": "true",
				  "DD_JMXFETCH_ENABLED": "true",
				  "DD_LOGS_CONFIG_USE_HTTP": "false",
				  "DD_LOGS_ENABLED": "true",
				  "DD_LOG_LEVEL": "Info",
				  "DD_SITE": "datadoghq.com",
				  "RUN_AGENT": "true",
				  "STD_LOG_COLLECTION_PORT": "10514"
				},
				"memory": 100,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "557941f4-83d3-4978-b0ac-a239f9fe771b",
				"command": "ruby app.rb",
				"console": false,
				"debug": null,
				"staging_task_id": "f13167ab-0d52-419e-8917-ac15e96d9cdb",
				"package_state": "STAGED",
				"health_check_type": "port",
				"health_check_timeout": 100,
				"health_check_http_endpoint": "",
				"staging_failed_reason": null,
				"staging_failed_description": null,
				"diego": true,
				"docker_image": null,
				"docker_credentials": {
				  "username": null,
				  "password": null
				},
				"package_updated_at": "2019-10-08T21:11:24Z",
				"detected_start_command": "ruby app.rb",
				"enable_ssh": true,
				"ports": [
				  8080
				],
				"space_url": "/v2/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/routes",
				"events_url": "/v2/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/events",
				"service_bindings_url": "/v2/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/service_bindings",
				"route_mappings_url": "/v2/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/route_mappings"
			  }
			}
		  ]
		}
	`)))
	case "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f":
		// we imply "?inline-relations-depth=2" here
		rw.Write([]byte(fmt.Sprintf(`
		{
			"metadata": {
				"guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f",
				"url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f",
				"created_at": "2019-05-17T15:06:02Z",
				"updated_at": "2019-10-04T11:11:00Z"
			},
			"entity": {
				"name": "p-invitations-green",
				"production": false,
				"space_guid": "417b893e-291e-48ec-94c7-7b2348604365",
				"space": {
					"metadata": {
						"guid": "417b893e-291e-48ec-94c7-7b2348604365"
					},
					"entity": {
						"name": "system",
						"organization": {
							"metadata": {
								"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
							},
							"entity": {
								"name": "system"
							}
						}
					}
				},
				"stack_guid": "b903564c-61ab-4555-bb01-5ed167db6e64",
				"buildpack": "nodejs_buildpack",
				"detected_buildpack": "nodejs",
				"detected_buildpack_guid": "26c169f4-0e24-49a6-a60c-b16ce5eea318",
				"environment_json": {
					"CLOUD_CONTROLLER_URL": "https://cloudfoundry.env",
					"COMPANY_NAME": "Pivotal",
					"INVITATIONS_CLIENT_ID": "invitations",
					"NODE_TLS_REJECT_UNAUTHORIZED": "0",
					"PRODUCT_NAME": "Apps Manager",
					"SUCCESS_CALLBACK_URL": "",
					"SUPPORT_EMAIL": ""
				},
				"memory": 256,
				"instances": 1,
				"disk_quota": 1024,
				"state": "STARTED",
				"version": "49479c78-27de-4398-ad37-b715f265ac4c",
				"command": null,
				"console": false,
				"debug": null,
				"staging_task_id": "ef7cfe60-d758-4a90-9113-4a2f852265b4",
				"package_state": "STAGED",
				"health_check_type": "port",
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
				"package_updated_at": "2019-10-04T11:10:26Z",
				"detected_start_command": "npm start",
				"enable_ssh": true,
				"ports": [
					8080
				],
				"space_url": "/v2/spaces/417b893e-291e-48ec-94c7-7b2348604365",
				"stack_url": "/v2/stacks/b903564c-61ab-4555-bb01-5ed167db6e64",
				"routes_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/routes",
				"events_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/events",
				"service_bindings_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/service_bindings",
				"route_mappings_url": "/v2/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/route_mappings"
			}
		}`)))
	case "/v3/apps":
		switch r.URL.Query().Get("page") {
		case "", "1":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
							"total_results": 14,
							"total_pages": 2,
							"first": {
								"href": "https://cloudfoundry.env/v3/apps?page=1&per_page=50"
							},
							"last": {
								"href": "https://cloudfoundry.env/v3/apps?page=2&per_page=50"
							},
							"next": {
								"href": "https://cloudfoundry.env/v3/apps?page=2&per_page=50"
							},
							"previous": null
						},
						"resources": [
							{
								"guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f",
								"name": "p-invitations-green",
								"state": "STARTED",
								"created_at": "2019-05-17T15:06:02Z",
								"updated_at": "2019-10-04T11:10:52Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"nodejs_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"metadata": {
									"annotations": {
										"app-annotation": "app-annotation-value",
										"app-space-annotation": "app-space-annotation-app-value",
										"app-org-annotation": "app-org-annotation-app-value",
										"app-space-org-annotation": "app-space-org-annotation-app-value"
									},
									"labels": {
										"app-label": "app-label-value",
										"app-space-label": "app-space-label-app-value",
										"app-org-label": "app-org-label-app-value",
										"app-space-org-label": "app-space-org-label-app-value"
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "487945cb-c486-4f7b-b313-139a0a686d31",
								"name": "nfsbroker",
								"state": "STARTED",
								"created_at": "2019-05-17T15:12:21Z",
								"updated_at": "2019-10-04T11:19:15Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "6f1fbbf4-b04c-4574-a7be-cb059e170287",
								"name": "gcp-service-broker-4.2.2",
								"state": "STARTED",
								"created_at": "2019-05-17T15:23:05Z",
								"updated_at": "2019-05-23T15:51:45Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"go_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417ca75c-3fea-4ea2-8428-b02bdf05deb0"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417ca75c-3fea-4ea2-8428-b02bdf05deb0"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "8054a565-d476-4535-807c-57e311da5051",
								"name": "hello-datadog-cf-ruby",
								"state": "STARTED",
								"created_at": "2019-05-21T12:15:09Z",
								"updated_at": "2019-09-03T08:52:23Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack",
											"datadog-cloudfoundry-buildpack",
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
								"name": "hello-datadog-cf-ruby-dev",
								"state": "STOPPED",
								"created_at": "2019-08-29T22:05:40Z",
								"updated_at": "2019-08-29T22:07:10Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack",
											"datadog-cloudfoundry-buildpack-dev",
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "827da8e5-1676-42ec-9028-46fbfe04fb86"
										}
									}
								},
								"metadata": {
									"annotations": {
										"app-annotation": "app-annotation-value",
										"app-space-annotation": "app-space-annotation-app-value",
										"app-org-annotation": "app-org-annotation-app-value",
										"app-space-org-annotation": "app-space-org-annotation-app-value",
										"blacklisted_key": "bar",
										"tags.datadoghq.com/auto-annotation-tag": "auto-annotation-tag-value"
									},
									"labels": {
										"app-label": "app-label-value",
										"app-space-label": "app-space-label-app-value",
										"app-org-label": "app-org-label-app-value",
										"app-space-org-label": "app-space-org-label-app-value",
										"blacklisted_key": "bar",
										"tags.datadoghq.com/auto-label-tag": "auto-label-tag-value"
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "771b41ca-d38f-4f4c-817d-80e5df4b11e0",
								"name": "hello-datadog-cf-ruby-dev-nick",
								"state": "STARTED",
								"created_at": "2019-08-30T21:59:08Z",
								"updated_at": "2019-09-18T17:04:00Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack",
											"datadog-cloudfoundry-buildpack-dev",
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "827da8e5-1676-42ec-9028-46fbfe04fb86"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "2697a7af-4190-402a-b3b7-de31c6065328",
								"name": "app-usage-scheduler",
								"state": "STARTED",
								"created_at": "2019-10-04T11:06:31Z",
								"updated_at": "2019-10-04T11:09:06Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "aa2431c4-9536-4f87-b78c-9d5a44717431",
								"name": "app-usage-worker",
								"state": "STARTED",
								"created_at": "2019-10-04T11:06:31Z",
								"updated_at": "2019-10-04T11:09:05Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "c482ca96-ac53-455e-9e29-a5cff5ef176a",
								"name": "app-usage-server",
								"state": "STARTED",
								"created_at": "2019-10-04T11:06:31Z",
								"updated_at": "2019-10-04T11:09:06Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"ruby_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "74e907a6-690a-4310-8804-c0b3beb5d302",
								"name": "apps-manager-js-green",
								"state": "STARTED",
								"created_at": "2019-10-04T11:10:26Z",
								"updated_at": "2019-10-04T11:10:44Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"staticfile_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "417b893e-291e-48ec-94c7-7b2348604365"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "46592861-ab1b-4088-ba13-9e09038d0054",
								"name": "notifications-ui",
								"state": "STARTED",
								"created_at": "2019-10-04T11:12:26Z",
								"updated_at": "2019-10-04T11:12:49Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "b096b43f-3c4f-4b5d-94a6-556cfcca17c0",
								"name": "autoscale",
								"state": "STARTED",
								"created_at": "2019-10-04T11:13:25Z",
								"updated_at": "2019-10-04T11:13:42Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"binary_buildpack"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/actions/stop",
										"method": "POST"
									}
								}
							},
							{
								"guid": "6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04",
								"name": "autoscale-api",
								"state": "STARTED",
								"created_at": "2019-10-04T11:13:43Z",
								"updated_at": "2019-10-04T11:15:10Z",
								"lifecycle": {
									"type": "buildpack",
									"data": {
										"buildpacks": [
											"java_buildpack_offline"
										],
										"stack": "cflinuxfs3"
									}
								},
								"relationships": {
									"space": {
										"data": {
											"guid": "d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
										}
									}
								},
								"links": {
									"self": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04"
									},
									"environment_variables": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/environment_variables"
									},
									"space": {
										"href": "https://cloudfoundry.env/v3/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
									},
									"processes": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/processes"
									},
									"route_mappings": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/route_mappings"
									},
									"packages": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/packages"
									},
									"current_droplet": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/droplets/current"
									},
									"droplets": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/droplets"
									},
									"tasks": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/tasks"
									},
									"start": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/actions/start",
										"method": "POST"
									},
									"stop": {
										"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/actions/stop",
										"method": "POST"
									}
								}
							}
						]
			}
		`)))
		case "2":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
					"total_results": 14,
					"total_pages": 2,
					"first": {
						"href": "https://cloudfoundry.env/v3/apps?page=1&per_page=50"
					},
					"last": {
						"href": "https://cloudfoundry.env/v3/apps?page=2&per_page=50"
					},
					"previous": {
						"href": "https://cloudfoundry.env/v3/apps?page=1&per_page=50"
					},
					"next": null
				},
				"resources": [
					{
						"guid": "7604d784-6ada-4b13-8a22-d892d8fa972d",
						"name": "hello-datadog-cf-ruby-dev-nick-no-python",
						"state": "STARTED",
						"created_at": "2019-10-08T20:01:39Z",
						"updated_at": "2019-10-08T21:12:29Z",
						"lifecycle": {
							"type": "buildpack",
							"data": {
								"buildpacks": [
									"binary_buildpack",
									"datadog-cloudfoundry-buildpack-no-python",
									"ruby_buildpack"
								],
								"stack": "cflinuxfs3"
							}
						},
						"relationships": {
							"space": {
								"data": {
									"guid": "827da8e5-1676-42ec-9028-46fbfe04fb86"
								}
							}
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d"
							},
							"environment_variables": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/environment_variables"
							},
							"space": {
								"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
							},
							"processes": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/processes"
							},
							"route_mappings": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/route_mappings"
							},
							"packages": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/packages"
							},
							"current_droplet": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/droplets/current"
							},
							"droplets": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/droplets"
							},
							"tasks": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/tasks"
							},
							"start": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/actions/start",
								"method": "POST"
							},
							"stop": {
								"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d/actions/stop",
								"method": "POST"
							}
						}
					}
				]
			}
			`)))
		}
	case "/v3/processes":
		switch r.URL.Query().Get("page") {
		case "", "1":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
				"total_results": 19,
				"total_pages": 2,
				"first": {
					"href": "https://cloudfoundry.env/v3/processes?page=1&per_page=50"
				},
				"last": {
					"href": "https://cloudfoundry.env/v3/processes?page=2&per_page=50"
				},
				"next": {
					"href": "https://cloudfoundry.env/v3/processes?page=2&per_page=50"
				},
				"previous": null
				},
				"resources": [
				{
					"guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 256,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-05-17T15:06:02Z",
					"updated_at": "2019-10-04T11:11:00Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/6d254438-cc3b-44a6-b2e6-343ca92deb5f"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/6d254438-cc3b-44a6-b2e6-343ca92deb5f/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/6d254438-cc3b-44a6-b2e6-343ca92deb5f/stats"
					}
					}
				},
				{
					"guid": "487945cb-c486-4f7b-b313-139a0a686d31",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 256,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-05-17T15:12:21Z",
					"updated_at": "2019-10-04T11:19:06Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/487945cb-c486-4f7b-b313-139a0a686d31"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/487945cb-c486-4f7b-b313-139a0a686d31/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/487945cb-c486-4f7b-b313-139a0a686d31"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/487945cb-c486-4f7b-b313-139a0a686d31/stats"
					}
					}
				},
				{
					"guid": "6f1fbbf4-b04c-4574-a7be-cb059e170287",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-05-17T15:23:05Z",
					"updated_at": "2019-05-23T15:50:16Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/6f1fbbf4-b04c-4574-a7be-cb059e170287"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/6f1fbbf4-b04c-4574-a7be-cb059e170287/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/6f1fbbf4-b04c-4574-a7be-cb059e170287"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417ca75c-3fea-4ea2-8428-b02bdf05deb0"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/6f1fbbf4-b04c-4574-a7be-cb059e170287/stats"
					}
					}
				},
				{
					"guid": "8054a565-d476-4535-807c-57e311da5051",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 100,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-05-21T12:15:09Z",
					"updated_at": "2019-09-03T08:51:08Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/8054a565-d476-4535-807c-57e311da5051"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/8054a565-d476-4535-807c-57e311da5051/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/8054a565-d476-4535-807c-57e311da5051"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/8054a565-d476-4535-807c-57e311da5051/stats"
					}
					}
				},
				{
					"guid": "80f19b75-3bfc-46b9-8d7b-b413d1c2267e",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 256,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-05-21T13:52:16Z",
					"updated_at": "2019-10-04T11:11:01Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/80f19b75-3bfc-46b9-8d7b-b413d1c2267e"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/80f19b75-3bfc-46b9-8d7b-b413d1c2267e/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/80f19b75-3bfc-46b9-8d7b-b413d1c2267e"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/80f19b75-3bfc-46b9-8d7b-b413d1c2267e/stats"
					}
					}
				},
				{
					"guid": "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 1000,
					"disk_in_mb": 2000,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-08-29T22:05:40Z",
					"updated_at": "2019-08-29T22:07:10Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/stats"
					}
					}
				},
				{
					"guid": "771b41ca-d38f-4f4c-817d-80e5df4b11e0",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 100,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-08-30T21:59:08Z",
					"updated_at": "2019-09-18T17:03:33Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/771b41ca-d38f-4f4c-817d-80e5df4b11e0"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/771b41ca-d38f-4f4c-817d-80e5df4b11e0/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/771b41ca-d38f-4f4c-817d-80e5df4b11e0"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/771b41ca-d38f-4f4c-817d-80e5df4b11e0/stats"
					}
					}
				},
				{
					"guid": "b04d2676-ce47-4ad4-bf5c-0b4ffe134556",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 2048,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "process",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-09-09T18:13:41Z",
					"updated_at": "2019-10-04T11:08:54Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/b04d2676-ce47-4ad4-bf5c-0b4ffe134556"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/b04d2676-ce47-4ad4-bf5c-0b4ffe134556"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/b04d2676-ce47-4ad4-bf5c-0b4ffe134556/stats"
					}
					}
				},
				{
					"guid": "60bc310d-fcd3-402f-b746-e014fc89cf79",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "process",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-09-09T18:13:41Z",
					"updated_at": "2019-10-04T11:08:54Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/60bc310d-fcd3-402f-b746-e014fc89cf79"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/60bc310d-fcd3-402f-b746-e014fc89cf79/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/60bc310d-fcd3-402f-b746-e014fc89cf79"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/60bc310d-fcd3-402f-b746-e014fc89cf79/stats"
					}
					}
				},
				{
					"guid": "2f064f6e-25ca-48a9-b503-42e18647a4f5",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 2,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "http",
					"data": {
						"timeout": 180,
						"invocation_timeout": null,
						"endpoint": "/heartbeat/db"
					}
					},
					"created_at": "2019-09-09T18:13:41Z",
					"updated_at": "2019-10-04T11:08:53Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/2f064f6e-25ca-48a9-b503-42e18647a4f5"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/2f064f6e-25ca-48a9-b503-42e18647a4f5/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/2f064f6e-25ca-48a9-b503-42e18647a4f5"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/2f064f6e-25ca-48a9-b503-42e18647a4f5/stats"
					}
					}
				},
				{
					"guid": "a0d5bb12-8848-43ca-a21a-c05912041e39",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 2,
					"memory_in_mb": 128,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-09-09T18:16:49Z",
					"updated_at": "2019-10-04T11:10:50Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/a0d5bb12-8848-43ca-a21a-c05912041e39"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/a0d5bb12-8848-43ca-a21a-c05912041e39/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/a0d5bb12-8848-43ca-a21a-c05912041e39"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/a0d5bb12-8848-43ca-a21a-c05912041e39/stats"
					}
					}
				},
				{
					"guid": "2697a7af-4190-402a-b3b7-de31c6065328",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "process",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:06:31Z",
					"updated_at": "2019-10-04T11:09:06Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/2697a7af-4190-402a-b3b7-de31c6065328"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/2697a7af-4190-402a-b3b7-de31c6065328/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/2697a7af-4190-402a-b3b7-de31c6065328"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/2697a7af-4190-402a-b3b7-de31c6065328/stats"
					}
					}
				},
				{
					"guid": "aa2431c4-9536-4f87-b78c-9d5a44717431",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 2048,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "process",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:06:31Z",
					"updated_at": "2019-10-04T11:09:05Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/aa2431c4-9536-4f87-b78c-9d5a44717431"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/aa2431c4-9536-4f87-b78c-9d5a44717431/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/aa2431c4-9536-4f87-b78c-9d5a44717431"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/aa2431c4-9536-4f87-b78c-9d5a44717431/stats"
					}
					}
				},
				{
					"guid": "c482ca96-ac53-455e-9e29-a5cff5ef176a",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 2,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "http",
					"data": {
						"timeout": 180,
						"invocation_timeout": null,
						"endpoint": "/heartbeat/db"
					}
					},
					"created_at": "2019-10-04T11:06:31Z",
					"updated_at": "2019-10-04T11:09:06Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/c482ca96-ac53-455e-9e29-a5cff5ef176a"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/c482ca96-ac53-455e-9e29-a5cff5ef176a/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/c482ca96-ac53-455e-9e29-a5cff5ef176a"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/c482ca96-ac53-455e-9e29-a5cff5ef176a/stats"
					}
					}
				},
				{
					"guid": "74e907a6-690a-4310-8804-c0b3beb5d302",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 2,
					"memory_in_mb": 128,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:10:26Z",
					"updated_at": "2019-10-04T11:10:50Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/74e907a6-690a-4310-8804-c0b3beb5d302"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/74e907a6-690a-4310-8804-c0b3beb5d302/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/74e907a6-690a-4310-8804-c0b3beb5d302"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/74e907a6-690a-4310-8804-c0b3beb5d302/stats"
					}
					}
				},
				{
					"guid": "46592861-ab1b-4088-ba13-9e09038d0054",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 2,
					"memory_in_mb": 64,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:12:26Z",
					"updated_at": "2019-10-04T11:12:49Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/46592861-ab1b-4088-ba13-9e09038d0054"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/46592861-ab1b-4088-ba13-9e09038d0054/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/46592861-ab1b-4088-ba13-9e09038d0054"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/46592861-ab1b-4088-ba13-9e09038d0054/stats"
					}
					}
				},
				{
					"guid": "b096b43f-3c4f-4b5d-94a6-556cfcca17c0",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 3,
					"memory_in_mb": 256,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": null,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:13:25Z",
					"updated_at": "2019-10-04T11:13:42Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/b096b43f-3c4f-4b5d-94a6-556cfcca17c0"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/b096b43f-3c4f-4b5d-94a6-556cfcca17c0"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/b096b43f-3c4f-4b5d-94a6-556cfcca17c0/stats"
					}
					}
				},
				{
					"guid": "6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 1024,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": 120,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-04T11:13:43Z",
					"updated_at": "2019-10-04T11:15:10Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/6d6a47f3-de72-4d44-8ab1-a1a3d19b9f04/stats"
					}
					}
				}
				]
			}`)))
		case "2":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
					"total_results": 19,
					"total_pages": 2,
					"first": {
						"href": "https://cloudfoundry.env/v3/processes?page=1&per_page=50"
					},
					"last": {
						"href": "https://cloudfoundry.env/v3/processes?page=2&per_page=50"
					},
					"previous": {
						"href": "https://cloudfoundry.env/v3/processes?page=1&per_page=50"
					},
					"next": null
				},
				"resources": [
				{
					"guid": "7604d784-6ada-4b13-8a22-d892d8fa972d",
					"type": "web",
					"command": "[PRIVATE DATA HIDDEN IN LISTS]",
					"instances": 1,
					"memory_in_mb": 100,
					"disk_in_mb": 1024,
					"health_check": {
					"type": "port",
					"data": {
						"timeout": 100,
						"invocation_timeout": null
					}
					},
					"created_at": "2019-10-08T20:01:39Z",
					"updated_at": "2019-10-08T21:11:28Z",
					"links": {
					"self": {
						"href": "https://cloudfoundry.env/v3/processes/7604d784-6ada-4b13-8a22-d892d8fa972d"
					},
					"scale": {
						"href": "https://cloudfoundry.env/v3/processes/7604d784-6ada-4b13-8a22-d892d8fa972d/actions/scale",
						"method": "POST"
					},
					"app": {
						"href": "https://cloudfoundry.env/v3/apps/7604d784-6ada-4b13-8a22-d892d8fa972d"
					},
					"space": {
						"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
					},
					"stats": {
						"href": "https://cloudfoundry.env/v3/processes/7604d784-6ada-4b13-8a22-d892d8fa972d/stats"
					}
					}
				}
			]
		}`)))
		}
	case "/v3/spaces":
		switch r.URL.Query().Get("page") {
		case "", "1":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
					"total_results": 6,
					"total_pages": 2,
					"first": {
						"href": "https://cloudfoundry.env/v3/spaces?page=1&per_page=50"
					},
					"last": {
						"href": "https://cloudfoundry.env/v3/spaces?page=2&per_page=50"
					},
					"next": {
						"href": "https://cloudfoundry.env/v3/spaces?page=2&per_page=50"
					},
					"previous": null
				},
				"resources": [
				{
					"guid": "417b893e-291e-48ec-94c7-7b2348604365",
					"created_at": "2019-05-17T15:02:37Z",
					"updated_at": "2019-05-17T15:02:37Z",
					"name": "system",
					"relationships": {
						"organization": {
							"data": {
							"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					},
					"metadata": {
						"annotations": {
							"space-annotation": "space-annotation-value",
							"app-space-annotation": "app-space-annotation-space-value",
							"space-org-annotation": "space-org-annotation-space-value",
							"app-space-org-annotation": "app-space-org-annotation-space-value"
						},
						"labels": {
							"space-label": "space-label-value",
							"app-space-label": "app-space-label-space-value",
							"space-org-label": "space-org-label-space-value",
							"app-space-org-label": "app-space-org-label-space-value"
						}
					},
					"links": {
						"self": {
							"href": "https://cloudfoundry.env/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365"
						},
						"organization": {
							"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
						}
					}
				},
				{
					"guid": "1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289",
					"created_at": "2019-05-17T15:07:05Z",
					"updated_at": "2019-05-17T15:07:05Z",
					"name": "notifications-with-ui",
					"relationships": {
						"organization": {
							"data": {
							"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					},
					"links": {
						"self": {
							"href": "https://cloudfoundry.env/v3/spaces/1b8dcf2e-ed92-4daa-b9fb-0fa5a97b9289"
						},
						"organization": {
							"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
						}
					}
				},
				{
					"guid": "d5d005a4-0320-4daa-ac0a-81f8dcd00fe0",
					"created_at": "2019-05-17T15:07:43Z",
					"updated_at": "2019-05-17T15:07:43Z",
					"name": "autoscaling",
					"relationships": {
						"organization": {
							"data": {
							"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					},
					"links": {
						"self": {
							"href": "https://cloudfoundry.env/v3/spaces/d5d005a4-0320-4daa-ac0a-81f8dcd00fe0"
						},
						"organization": {
							"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
						}
					}
				}
				]
			}`)))
		case "2":
			rw.Write([]byte(fmt.Sprintf(`
				{
					"pagination": {
						"total_results": 6,
						"total_pages": 2,
						"first": {
							"href": "https://cloudfoundry.env/v3/spaces?page=1&per_page=50"
						},
						"last": {
							"href": "https://cloudfoundry.env/v3/spaces?page=2&per_page=50"
						},
						"next": null,
						"previous": {
							"href": "https://cloudfoundry.env/v3/spaces?page=1&per_page=50"
						}
					},
					"resources": [
					{
						"guid": "8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793",
						"created_at": "2019-05-17T15:12:19Z",
						"updated_at": "2019-05-17T15:12:19Z",
						"name": "nfs",
						"relationships": {
							"organization": {
								"data": {
								"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
								}
							}
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/spaces/8c7e64bb-0bf8-4a7a-92e1-2fe06e7ec793"
							},
							"organization": {
								"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					},
					{
						"guid": "417ca75c-3fea-4ea2-8428-b02bdf05deb0",
						"created_at": "2019-05-17T15:23:03Z",
						"updated_at": "2019-05-17T15:23:03Z",
						"name": "gcp-service-broker-space",
						"relationships": {
							"organization": {
								"data": {
								"guid": "671557cf-edcd-49df-9863-ee14513d13c7"
								}
							}
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/spaces/417ca75c-3fea-4ea2-8428-b02bdf05deb0"
							},
							"organization": {
								"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					},
					{
						"guid": "827da8e5-1676-42ec-9028-46fbfe04fb86",
						"created_at": "2019-05-21T09:42:46Z",
						"updated_at": "2019-05-21T09:42:46Z",
						"name": "datadog-application-monitoring-space",
						"relationships": {
							"organization": {
								"data": {
								"guid": "8c19a50e-7974-4c67-adea-9640fae21526"
								}
							}
						},
						"metadata": {
							"annotations": {
								"space-annotation": "space-annotation-value",
								"app-space-annotation": "app-space-annotation-space-value",
								"space-org-annotation": "space-org-annotation-space-value",
								"app-space-org-annotation": "app-space-org-annotation-space-value"
							},
							"labels": {
								"space-label": "space-label-value",
								"app-space-label": "app-space-label-space-value",
								"space-org-label": "space-org-label-space-value",
								"app-space-org-label": "app-space-org-label-space-value"
							}
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/spaces/827da8e5-1676-42ec-9028-46fbfe04fb86"
							},
							"organization": {
								"href": "https://cloudfoundry.env/v3/organizations/8c19a50e-7974-4c67-adea-9640fae21526"
							}
						}
					}
					]
				}`)))
		}
	case "/v2/quota_definitions":
		rw.Write([]byte(fmt.Sprintf(`{
			"total_results": 2,
			"total_pages": 1,
			"prev_url": null,
			"next_url": null,
			"resources": [
			   {
				  "metadata": {
					 "guid": "1345a873-ead6-4ae2-9d7d-e270c1a05c82",
					 "url": "/v2/quota_definitions/1345a873-ead6-4ae2-9d7d-e270c1a05c82",
					 "created_at": "2019-05-17T13:06:27Z",
					 "updated_at": "2019-05-17T13:06:27Z"
				  },
				  "entity": {
					 "name": "default",
					 "non_basic_services_allowed": true,
					 "total_services": 100,
					 "total_routes": 1000,
					 "total_private_domains": -1,
					 "memory_limit": 10240,
					 "trial_db_allowed": false,
					 "instance_memory_limit": -1,
					 "app_instance_limit": -1,
					 "app_task_limit": -1,
					 "total_service_keys": -1,
					 "total_reserved_route_ports": 0
				  }
			   },
			   {
				  "metadata": {
					 "guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e",
					 "url": "/v2/quota_definitions/1cf98856-aba8-49a8-8b21-d82a25898c4e",
					 "created_at": "2019-05-17T13:06:27Z",
					 "updated_at": "2019-05-17T13:06:27Z"
				  },
				  "entity": {
					 "name": "runaway",
					 "non_basic_services_allowed": true,
					 "total_services": -1,
					 "total_routes": 1000,
					 "total_private_domains": -1,
					 "memory_limit": 102400,
					 "trial_db_allowed": false,
					 "instance_memory_limit": -1,
					 "app_instance_limit": -1,
					 "app_task_limit": -1,
					 "total_service_keys": -1,
					 "total_reserved_route_ports": 0
				  }
			   }
			]
		}`)))
	case "/v2/organizations":
		rw.Write([]byte(fmt.Sprintf(`{
			"total_results": 2,
			"total_pages": 1,
			"prev_url": null,
			"next_url": null,
			"resources": [
				{
					"metadata": {
						"guid": "671557cf-edcd-49df-9863-ee14513d13c7",
						"url": "/v2/organizations/671557cf-edcd-49df-9863-ee14513d13c7",
						"created_at": "2019-05-17T13:06:27Z",
						"updated_at": "2019-10-04T11:10:22Z"
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
				{
					"metadata": {
						"guid": "8c19a50e-7974-4c67-adea-9640fae21526",
						"url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526",
						"created_at": "2019-05-21T09:42:45Z",
						"updated_at": "2019-05-21T09:42:45Z"
					},
					"entity": {
						"name": "datadog-application-monitoring-org",
						"billing_enabled": false,
						"quota_definition_guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e",
						"status": "active",
						"default_isolation_segment_guid": null,
						"quota_definition_url": "/v2/quota_definitions/1cf98856-aba8-49a8-8b21-d82a25898c4e",
						"spaces_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/spaces",
						"domains_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/domains",
						"private_domains_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/private_domains",
						"users_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/users",
						"managers_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/managers",
						"billing_managers_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/billing_managers",
						"auditors_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/auditors",
						"app_events_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/app_events",
						"space_quota_definitions_url": "/v2/organizations/8c19a50e-7974-4c67-adea-9640fae21526/space_quota_definitions"
					}
				}
			]
			}`)))
	case "/v3/organizations":
		switch r.URL.Query().Get("page") {
		case "", "1":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
					"total_results": 2,
					"total_pages": 2,
					"first": {
						"href": "https://cloudfoundry.env/v3/organizations?page=1&per_page=50"
					},
					"last": {
						"href": "https://cloudfoundry.env/v3/organizations?page=2&per_page=50"
					},
					"previous": null,
					"next": {
						"href": "https://cloudfoundry.env/v3/organizations?page=2&per_page=50"
					}
				},
				"resources": [
					{
						"guid": "671557cf-edcd-49df-9863-ee14513d13c7",
						"name": "system",
						"created_at": "2019-05-17T13:06:27Z",
						"updated_at": "2019-10-04T11:10:22Z",
						"relationships": {
							"quota": {
								"data": {
									"guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e"
								}
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
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7"
							}
						}
					}
				]
			}`)))
		case "2":
			rw.Write([]byte(fmt.Sprintf(`
			{
				"pagination": {
					"total_results": 2,
					"total_pages": 2,
					"first": {
						"href": "https://cloudfoundry.env/v3/organizations?page=1&per_page=50"
					},
					"last": {
						"href": "https://cloudfoundry.env/v3/organizations?page=2&per_page=50"
					},
					"next": null,
					"previous": {
						"href": "https://cloudfoundry.env/v3/organizations?page=1&per_page=50"
					}
				},
				"resources": [
					{
						"guid": "8c19a50e-7974-4c67-adea-9640fae21526",
						"name": "datadog-application-monitoring-org",
						"updated_at": "2019-10-04T11:10:22Z",
						"relationships": {
							"quota": {
								"data": {
									"guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e"
								}
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
						},
						"links": {
							"self": {
								"href": "https://cloudfoundry.env/v3/organizations/8c19a50e-7974-4c67-adea-9640fae21526"
							}
						}
					}
		  		]
			}`)))
		}
	case "/oauth/token":
		rw.Write([]byte(fmt.Sprintf(`
		{
			"token_type": "%s",
			"access_token": "%s"
		}
		`, f.tokenType, f.accessToken)))
	case "/v2/read":
		http.Redirect(rw, r, "http://asdasdasd.com", http.StatusPermanentRedirect)
	default:
		if strings.Contains(r.URL.Path, "/sidecars") {
			if strings.Contains(r.URL.Path, "6d254438-cc3b-44a6-b2e6-343ca92deb5f") {
				rw.Write([]byte(`
			{
				"pagination": {
				   "total_results": 1,
				   "total_pages": 1,
				   "first": {
					  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/sidecars?page=1&per_page=50"
				   },
				   "last": {
					  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/sidecars?page=1&per_page=50"
				   },
				   "next": null,
				   "previous": null
				},
				"resources": [
				   {
					  "guid": "68a03f42-5392-47ed-9979-477d48a61927",
					  "name": "config-server",
					  "command": "./config-server",
					  "process_types": [
						 "web"
					  ],
					  "memory_in_mb": null,
					  "origin": "user",
					  "relationships": {
						 "app": {
							"data": {
							   "guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f"
							}
						 }
					  },
					  "created_at": "2022-03-17T09:57:48Z",
					  "updated_at": "2022-03-17T09:57:48Z"
				   }
				]
			 }
			`))
			} else if strings.Contains(r.URL.Path, "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a") {
				rw.Write([]byte(`
				{
					"pagination": {
					   "total_results": 1,
					   "total_pages": 1,
					   "first": {
						  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/sidecars?page=1&per_page=50"
					   },
					   "last": {
						  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a/sidecars?page=1&per_page=50"
					   },
					   "next": null,
					   "previous": null
					},
					"resources": [
					   {
						  "guid": "sidecar_guid",
						  "name": "sidecar_name",
						  "command": "sidecar_command",
						  "process_types": [
							 "process_type_1"
						  ],
						  "memory_in_mb": null,
						  "origin": "user",
						  "relationships": {
							 "app": {
								"data": {
								   "guid": "6116f9ec-2bd6-4dd6-b7fe-a1b6acf6662a"
								}
							 }
						  },
						  "created_at": "2022-03-17T09:57:48Z",
						  "updated_at": "2022-03-17T09:57:48Z"
					   }
					]
				 }
				`))
			} else {
				rw.Write([]byte(`
				{
					"pagination": {
					   "total_results": 0,
					   "total_pages": 1,
					   "first": {
						  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/ead4c7fd-f21c-48b8-9f23-421f15a57cfc/sidecars?page=1&per_page=50"
					   },
					   "last": {
						  "href": "https://api.sys.integrations-lab.devenv.dog/v3/apps/ead4c7fd-f21c-48b8-9f23-421f15a57cfc/sidecars?page=1&per_page=50"
					   },
					   "next": null,
					   "previous": null
					},
					"resources": []
				 }
				`))
			}
		}
	}
}

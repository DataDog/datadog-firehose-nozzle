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
	case "/":
		rw.Write([]byte(fmt.Sprintf(`
		{
			"links": {
				"self": {
					"href": "%[1]s"
				},
				"cloud_controller_v3": {
					"href": "%[1]s/v3",
					"meta": {
						"version": "3.150.0"
					}
				},
				"login": {
					"href": "%[1]s"
				},
				"uaa": {
					"href": "%[1]s"
				}
			}
		}
		`, f.URL())))
	case "/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f":
		rw.Write([]byte(`
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
			}
		}`))
	case "/v3/spaces/417b893e-291e-48ec-94c7-7b2348604365":
		rw.Write([]byte(`
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
			}
		}`))
	case "/v3/organizations/671557cf-edcd-49df-9863-ee14513d13c7":
		rw.Write([]byte(`
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
			}
		}`))
	case "/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/processes":
		rw.Write([]byte(`
		{
			"pagination": {
				"total_results": 1,
				"total_pages": 1,
				"first": {
					"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/processes?page=1&per_page=50"
				},
				"last": {
					"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f/processes?page=1&per_page=50"
				},
				"next": null,
				"previous": null
			},
			"resources": [
				{
					"guid": "6d254438-cc3b-44a6-b2e6-343ca92deb5f",
					"type": "web",
					"instances": 1,
					"memory_in_mb": 256,
					"disk_in_mb": 1024,
					"created_at": "2019-05-17T15:06:02Z",
					"updated_at": "2019-10-04T11:11:00Z",
					"links": {
						"app": {
							"href": "https://cloudfoundry.env/v3/apps/6d254438-cc3b-44a6-b2e6-343ca92deb5f"
						}
					}
				}
			]
		}`))
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
										"tags.datadoghq.com/ddsource": "ddsource-label-value",
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
	case "/v3/organization_quotas":
		rw.Write([]byte(`
		{
			"pagination": {
				"total_results": 3,
				"total_pages": 1,
				"first": {
					"href": "https://cloudfoundry.env/v3/organization_quotas?page=1&per_page=50"
				},
				"last": {
					"href": "https://cloudfoundry.env/v3/organization_quotas?page=1&per_page=50"
				},
				"next": null,
				"previous": null
			},
			"resources": [
				{
					"guid": "1345a873-ead6-4ae2-9d7d-e270c1a05c82",
					"created_at": "2019-05-17T13:06:27Z",
					"updated_at": "2019-05-17T13:06:27Z",
					"name": "default",
					"apps": {
						"total_memory_in_mb": 10240,
						"per_process_memory_in_mb": null,
						"total_instances": null,
						"per_app_tasks": null
					}
				},
				{
					"guid": "1cf98856-aba8-49a8-8b21-d82a25898c4e",
					"created_at": "2019-05-17T13:06:27Z",
					"updated_at": "2019-05-17T13:06:27Z",
					"name": "runaway",
					"apps": {
						"total_memory_in_mb": 102400,
						"per_process_memory_in_mb": null,
						"total_instances": null,
						"per_app_tasks": null
					}
				},
				{
					"guid": "5c1e3a3c-0b5e-4bd1-9e3a-6a3b1f6f0c11",
					"created_at": "2019-05-17T13:06:27Z",
					"updated_at": "2019-05-17T13:06:27Z",
					"name": "unlimited",
					"apps": {
						"total_memory_in_mb": null,
						"per_process_memory_in_mb": null,
						"total_instances": null,
						"per_app_tasks": null
					}
				}
			]
		}`))
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

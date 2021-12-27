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

	tokenType string
	authToken string

	requested bool

	closeMessage []byte

	// Used to make the controller slow to answer
	RequestTime time.Duration
}

// NewFakeClusterAgentAPI create a new cloud controller
func NewFakeClusterAgentAPI(tokenType string, authToken string) *FakeClusterAgentAPI {
	return &FakeClusterAgentAPI{
		ReceivedContents: make(chan []byte, 100),
		ReceivedRequests: make(chan *http.Request, 100),
		usedEndpoints:    []string{},
		tokenType:        tokenType,
		authToken:        authToken,
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
	if f.tokenType == "" && f.authToken == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", f.tokenType, f.authToken)
}

func (f *FakeClusterAgentAPI) writeResponse(rw http.ResponseWriter, r *http.Request) {
	// NOTE: app with GUID "6d254438-cc3b-44a6-b2e6-343ca92deb5f" is checked explicitly
	// in client_test.go, so any changes to this app and objects related to it will likely
	// result in failure of some of the tests in that file
	switch r.URL.Path {
	case "/v1/cf/apps":
		rw.Write([]byte("no response"))
	case "/v1/cf/processes":
		rw.Write([]byte("no response"))
	case "/v1/cf/spaces":
		rw.Write([]byte("no response"))
	case "/v1/cf/organizations":
		rw.Write([]byte("no response"))
	}
}

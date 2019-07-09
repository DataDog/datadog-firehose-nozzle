package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/cloudfoundry/sonde-go/events"
)

// FakeCloudControllerAPI mocks a cloud controller
type FakeCloudControllerAPI struct {
	ReceivedContents chan []byte
	ReceivedRequests chan cfclient.Request

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
		ReceivedRequests: make(chan cfclient.Request, 100),
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
	}()
	defer r.Body.Close()
	rw.Write([]byte(fmt.Sprintf(`
		{
			"token_type": "%s",
			"access_token": "%s"
		}
	`, f.tokenType, f.accessToken)))
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

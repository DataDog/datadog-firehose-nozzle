package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

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

	time.Sleep(f.RequestTime * time.Millisecond)
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
		file, _ := ioutil.ReadFile("../integration/testdata/v2info.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file), f.URL(), f.URL(), f.URL())))
	case "/v2/apps":
		params, _ := url.ParseQuery(r.URL.RawQuery)
		page := params["page"][0]
		file, _ := ioutil.ReadFile("../integration/testdata/v2apps.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file), f.AppNumber, f.AppNumber, page, page)))
	case "/v3/apps":
		file, _ := ioutil.ReadFile("../integration/testdata/v3apps.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file))))
	case "/v3/processes":
		file, _ := ioutil.ReadFile("../integration/testdata/v3processes.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file))))
	case "/v3/spaces":
		file, _ := ioutil.ReadFile("../integration/testdata/v3spaces.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file))))
	case "/v2/quota_definitions":
		file, _ := ioutil.ReadFile("../integration/testdata/v2quota_definitions.json")
		fmt.Println(string(file))
		rw.Write([]byte(fmt.Sprintf(string(file))))
	case "/oauth/token":
		rw.Write([]byte(fmt.Sprintf(`
		{
			"token_type": "%s",
			"access_token": "%s"
		}
		`, f.tokenType, f.accessToken)))
	}
}

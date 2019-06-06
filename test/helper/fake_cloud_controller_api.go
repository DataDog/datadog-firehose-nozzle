package helper

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/cloudfoundry/sonde-go/events"
)

type FakeCloudControllerAPI struct {
	ReceivedContents chan []byte

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

func NewFakeCloudControllerAPI(tokenType string, accessToken string) *FakeCloudControllerAPI {
	return &FakeCloudControllerAPI{
		ReceivedContents: make(chan []byte, 100),
		tokenType:        tokenType,
		accessToken:      accessToken,
	}
}

func (f *FakeCloudControllerAPI) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

func (f *FakeCloudControllerAPI) Close() {
	f.server.Close()
}

func (f *FakeCloudControllerAPI) URL() string {
	return f.server.URL
}

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

func (f *FakeCloudControllerAPI) AuthToken() string {
	if f.tokenType == "" && f.accessToken == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", f.tokenType, f.accessToken)
}

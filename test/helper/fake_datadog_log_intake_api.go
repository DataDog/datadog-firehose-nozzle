package helper

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
)

type FakeDatadogLogIntakeAPI struct {
	server           *httptest.Server
	ReceivedContents chan []byte
}

func NewFakeDatadogLogIntakeAPI() *FakeDatadogLogIntakeAPI {
	return &FakeDatadogLogIntakeAPI{
		ReceivedContents: make(chan []byte, 100),
	}
}

func (f *FakeDatadogLogIntakeAPI) Start(wg *sync.WaitGroup) {
	f.server = httptest.NewUnstartedServer(f)
	wg.Done()
	f.server.Start()
}

func (f *FakeDatadogLogIntakeAPI) Close() {
	f.server.CloseClientConnections()
	f.server.Close()
}

func (f *FakeDatadogLogIntakeAPI) URL() string {
	return f.server.URL
}

func (f *FakeDatadogLogIntakeAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	contents, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	go func() {
		f.ReceivedContents <- contents
	}()
}

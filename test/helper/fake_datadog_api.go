package helper

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
)

type FakeDatadogAPI struct {
	server           *httptest.Server
	ReceivedContents chan []byte
	ReceivedRequests chan *http.Request
	NextResponses    chan *FakeResponse
}

func NewFakeDatadogAPI() *FakeDatadogAPI {
	return &FakeDatadogAPI{
		ReceivedContents: make(chan []byte, 100),
		ReceivedRequests: make(chan *http.Request, 100),
		NextResponses:    make(chan *FakeResponse, 100),
	}
}

func (f *FakeDatadogAPI) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

func (f *FakeDatadogAPI) SetNextResponse(resp *FakeResponse) {
	f.NextResponses <- resp
}

func (f *FakeDatadogAPI) Close() {
	f.server.Close()
}

func (f *FakeDatadogAPI) URL() string {
	return f.server.URL
}

func (f *FakeDatadogAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	contents, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	f.ReceivedRequests <- r

	go func() {
		f.ReceivedContents <- contents
	}()

	select {
	case response := <-f.NextResponses:
		rw.WriteHeader(response.status)
		rw.Write(response.data)
		return
	default:
		return
	}
}

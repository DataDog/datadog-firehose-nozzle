package helper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

type FakeFirehose struct {
	server *httptest.Server
	closeServerLoop chan bool
	serveBatch chan *loggregator_v2.EnvelopeBatch
	lock   sync.Mutex

	validToken string

	lastAuthorization string
	requested         bool
	badRequest        bool

	events       []*loggregator_v2.Envelope
}

func NewFakeFirehose(validToken string) *FakeFirehose {
	return &FakeFirehose{
		validToken:   validToken,
		closeServerLoop: make(chan bool, 1),
		serveBatch:    make(chan *loggregator_v2.EnvelopeBatch, 1),
	}
}

func (f *FakeFirehose) Start() {
	f.server = httptest.NewUnstartedServer(f)
	f.server.Start()
}

func (f *FakeFirehose) Close() {
	f.closeServerLoop <- true
	f.server.Close()
}

func (f *FakeFirehose) CloseServerLoop() {
	f.closeServerLoop <- true
}

func (f *FakeFirehose) URL() string {
	return f.server.URL
}

func (f *FakeFirehose) LastAuthorization() string {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.lastAuthorization
}

func (f *FakeFirehose) Requested() bool {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.requested
}

func (f *FakeFirehose) AddEvent(event loggregator_v2.Envelope) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.events = append(f.events, &event)
}

func (f *FakeFirehose) ServeBatch() {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.serveBatch <- &loggregator_v2.EnvelopeBatch{
		Batch: f.events,
	}
	f.events = []*loggregator_v2.Envelope{}
}

func (f *FakeFirehose) writeBatch(rw http.ResponseWriter, batch *loggregator_v2.EnvelopeBatch) {
	f.lock.Lock()
	defer f.lock.Unlock()
	marshalledBatch, _ := marshaler.MarshalToString(batch)
	toWrite := fmt.Sprintf("data: %s\n\n", marshalledBatch)
	fmt.Fprint(rw, toWrite)
}

func (f *FakeFirehose) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	flusher, _ := rw.(http.Flusher)

	f.lock.Lock()
	f.lastAuthorization = r.Header.Get("Authorization")
	f.requested = true

	if f.lastAuthorization != f.validToken {
		rw.WriteHeader(403)
		time.Sleep(100 * time.Millisecond)
		f.lock.Unlock()
		return
	}
	f.lock.Unlock()

	for {
		select {
		case b := <- f.serveBatch:
			f.writeBatch(rw, b)
			flusher.Flush()
		case <- f.closeServerLoop:
			return
		}
	}
}

func (f *FakeFirehose) SetToken(token string) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.validToken = token
}

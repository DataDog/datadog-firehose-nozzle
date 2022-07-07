package uaatokenfetcher

import (
	"sync"

	"github.com/cloudfoundry/gosteno"

	"github.com/DataDog/datadog-firehose-nozzle/test/helper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UaaTokenFetcher", func() {
	var (
		wg           *sync.WaitGroup
		tokenFetcher *UAATokenFetcher
		fakeUAA      *helper.FakeUAA
		fakeToken    string
		fakeLogger   *gosteno.Logger
	)

	BeforeEach(func() {
		fakeLogger = helper.Logger()
		fakeUAA = helper.NewFakeUAA("bearer", "123456789")
		fakeToken = fakeUAA.AuthToken()
		wg = &sync.WaitGroup{}
		wg.Add(1)
		fakeUAA.Start(wg)
		wg.Wait()

		tokenFetcher = New(fakeUAA.URL(), "username", "password", true, fakeLogger)
	})

	It("fetches a token from the UAA", func() {
		receivedAuthToken := tokenFetcher.FetchAuthToken()
		Expect(fakeUAA.Requested()).To(BeTrue())
		Expect(receivedAuthToken).To(Equal(fakeToken))
	})
})

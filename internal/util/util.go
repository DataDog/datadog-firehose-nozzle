package util

import (
	"crypto/sha1"
	"math/rand"
	"sort"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

func HashTags(tags []string) string {
	// This is the origial implementation of this function from the original nozzle
	// It might make sense to concat all tags together in the future
	sort.Strings(tags)
	hash := ""
	for _, tag := range tags {
		tagHash := sha1.Sum([]byte(tag))
		hash += string(tagHash[:])
	}
	return hash
}

func GetTickerWithJitter(wholeIntervalSeconds uint32, jitterPct float64) (*time.Ticker, func()) {
	wholeTick := int64(wholeIntervalSeconds) * int64(time.Second)
	shortenedTick := int64(float64(wholeTick) * (1.0 - jitterPct))
	jitterMax := int64(float64(wholeTick) * jitterPct)
	ticker := time.NewTicker(time.Duration(shortenedTick))
	jitterWait := func() {
		jitter := rand.Int63n(jitterMax)
		time.Sleep(time.Duration(jitter))
	}
	return ticker, jitterWait
}

func IsContainerMetric(envelope *loggregator_v2.Envelope) bool {
	result := false
	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Gauge:
		result = true
		// TOOD: verify that this is the right set of keys
		for _, key := range []string{"cpu", "memory", "disk", "memory_quota", "disk_quota"} {
			if _, ok := envelope.GetGauge().GetMetrics()[key]; !ok {
				result = false
			}
		}
	}

	return result
}
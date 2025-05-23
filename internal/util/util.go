package util

import (
	"crypto/sha1"
	"math/rand"
	"sort"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

var highCardinalityTags []string = []string{"created_at", "user_data"}

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

func FilterHighCardinalityTags(tags []string) []string {
	var filteredTags []string
	for _, tag := range tags {
		isAllowed := true
		for _, excludedTag := range highCardinalityTags {
			if strings.Contains(tag, excludedTag) {
				isAllowed = false
				break
			}
		}
		if isAllowed {
			filteredTags = append(filteredTags, tag)
		}
	}
	return filteredTags
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
	// We can tell whether or not a Gauge envelope is container metric by checking
	// a predefined set of metrics: https://github.com/cloudfoundry/loggregator-api#containermetric
	result := false
	switch envelope.GetMessage().(type) {
	case *loggregator_v2.Envelope_Gauge:
		result = true
		for _, key := range []string{"cpu", "memory", "disk", "memory_quota", "disk_quota"} {
			if v, ok := envelope.GetGauge().GetMetrics()[key]; !ok || v == nil || (v.Unit == "" && v.Value == 0) {
				result = false
			}
		}
	}

	return result
}

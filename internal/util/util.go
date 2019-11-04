package util

import (
	"crypto/sha1"
	"math/rand"
	"sort"
	"time"
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
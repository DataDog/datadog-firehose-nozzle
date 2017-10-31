package utils

import (
	"crypto/sha1"
	"sort"
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

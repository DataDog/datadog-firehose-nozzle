package utils

import (
	"crypto/sha1"
	"sort"
)

func HashTags(tags []string) string {
	sort.Strings(tags)
	hash := ""
	for _, tag := range tags {
		tagHash := sha1.Sum([]byte(tag))
		hash += string(tagHash[:])
	}
	return hash
}

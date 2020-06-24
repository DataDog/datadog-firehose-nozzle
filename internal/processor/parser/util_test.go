package parser

import (
	"regexp"

	"github.com/DataDog/datadog-firehose-nozzle/internal/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParserUtils", func() {

	Context("Metadata Key Filtering", func() {
		It("allows everything when empty", func() {
			config.NozzleConfig = config.Config{}
			Expect(isMetadataKeyAllowed("aRandomValue")).To(BeTrue())
		})

		It("reject strings matching a blacklist pattern", func() {
			config.NozzleConfig = config.Config{
				MetadataKeysBlacklist: []*regexp.Regexp{regexp.MustCompile("blacklisted.*")},
			}
			Expect(isMetadataKeyAllowed("aRandomValue")).To(BeTrue())
			Expect(isMetadataKeyAllowed("blacklistedValue")).To(BeFalse())
		})

		It("rejects strings not matching a whitelist pattern", func() {
			config.NozzleConfig = config.Config{
				MetadataKeysWhitelist: []*regexp.Regexp{regexp.MustCompile("whitelisted.*")},
			}
			Expect(isMetadataKeyAllowed("aRandomValue")).To(BeFalse())
			Expect(isMetadataKeyAllowed("whitelistedValue")).To(BeTrue())
		})

		It("rejects a value both blacklisted and whitelisted", func() {
			config.NozzleConfig = config.Config{
				MetadataKeysWhitelist: []*regexp.Regexp{regexp.MustCompile("whitelisted.*"), regexp.MustCompile("whitelistedAndBlacklisted.*")},
				MetadataKeysBlacklist: []*regexp.Regexp{regexp.MustCompile("blacklisted.*"), regexp.MustCompile("whitelistedAndBlacklisted.*")},
			}
			Expect(isMetadataKeyAllowed("blacklistedValue")).To(BeFalse())
			Expect(isMetadataKeyAllowed("whitelistedValue")).To(BeTrue())
			Expect(isMetadataKeyAllowed("whitelistedAndBlacklistedValue")).To(BeFalse())
		})
	})
})

package tags

import (
	"testing"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBoshTags(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bosh Tags Suite")
}

var _ = Describe("GenerateBoshTags", func() {
	var (
		cfg  BoshTagsConfig
		meta EnvelopeMetadata
	)

	BeforeEach(func() {
		cfg = BoshTagsConfig{}
		meta = EnvelopeMetadata{
			ID:         "abc-123",
			Job:        "diego-cell",
			Index:      "0",
			AZ:         "z1",
			Deployment: "cf-prod",
			Address:    "10.0.0.1",
			IP:         "10.0.0.2",
		}
	})

	It("returns only mandatory tags when disabled", func() {
		cfg.Enabled = false
		tags := GenerateBoshTags(cfg, meta)

		Expect(tags).To(ContainElement("bosh_deployment:cf-prod"))
		Expect(tags).To(ContainElement("deployment:cf-prod"))
		Expect(tags).NotTo(ContainElement("bosh_id:abc-123"))
		Expect(tags).NotTo(ContainElement("bosh_job:diego-cell"))
	})

	It("generates all prefixed tags when enabled", func() {
		cfg.Enabled = true
		tags := GenerateBoshTags(cfg, meta)

		Expect(tags).To(ContainElement("bosh_id:abc-123"))
		Expect(tags).To(ContainElement("bosh_job:diego-cell"))
		Expect(tags).To(ContainElement("bosh_name:diego-cell"))
		Expect(tags).To(ContainElement("bosh_index:0"))
		Expect(tags).To(ContainElement("bosh_az:z1"))
		Expect(tags).To(ContainElement("bosh_deployment:cf-prod"))
		// Always-added mandatory tags
		Expect(tags).To(ContainElement("deployment:cf-prod"))
	})

	It("always includes address and IP tags when enabled", func() {
		cfg.Enabled = true
		tags := GenerateBoshTags(cfg, meta)
		Expect(tags).To(ContainElement("bosh_address:10.0.0.1"))
		Expect(tags).To(ContainElement("bosh_ip:10.0.0.2"))
	})

	It("skips tags for empty metadata fields", func() {
		cfg.Enabled = true
		meta.AZ = ""
		meta.Address = ""
		meta.IP = ""
		tags := GenerateBoshTags(cfg, meta)
		Expect(tags).NotTo(ContainElement("bosh_az:"))
		Expect(tags).NotTo(ContainElement("bosh_address:"))
		Expect(tags).NotTo(ContainElement("bosh_ip:"))
	})

	It("does not duplicate bosh_deployment when enabled", func() {
		cfg.Enabled = true
		tags := GenerateBoshTags(cfg, meta)
		count := 0
		for _, t := range tags {
			if t == "bosh_deployment:cf-prod" {
				count++
			}
		}
		Expect(count).To(Equal(1))
	})
})

var _ = Describe("GenerateHostname", func() {
	var (
		cfg  BoshTagsConfig
		meta EnvelopeMetadata
	)

	BeforeEach(func() {
		cfg = BoshTagsConfig{}
		meta = EnvelopeMetadata{
			ID:         "abc-123",
			Job:        "diego-cell",
			Index:      "0",
			Deployment: "cf-prod",
		}
	})

	It("uses UUID hostname when configured", func() {
		cfg.UseUUIDHostname = true
		Expect(GenerateHostname(cfg, meta)).To(Equal("abc-123"))
	})

	It("returns friendly hostname (job-index)", func() {
		cfg.FriendlyHostname = true
		Expect(GenerateHostname(cfg, meta)).To(Equal("diego-cell-0"))
	})

	It("appends deployment for unique friendly hostname", func() {
		cfg.FriendlyHostname = true
		cfg.UniqueFriendlyHostname = true
		Expect(GenerateHostname(cfg, meta)).To(Equal("diego-cell-0-cf-prod"))
	})

	It("appends VM GUID when configured", func() {
		cfg.FriendlyHostname = true
		cfg.FriendlyHostnameAppendGUID = true
		Expect(GenerateHostname(cfg, meta)).To(Equal("diego-cell-0-abc-123"))
	})

	It("falls back to job-index when nothing configured", func() {
		Expect(GenerateHostname(cfg, meta)).To(Equal("diego-cell-0"))
	})

	It("replaces underscores in friendly hostnames", func() {
		cfg.FriendlyHostname = true
		meta.Job = "diego_cell"
		Expect(GenerateHostname(cfg, meta)).To(Equal("diego-cell-0"))
	})
})

var _ = Describe("ExtractMetadataFromEnvelope", func() {
	It("extracts all fields from envelope tags", func() {
		envelope := &loggregator_v2.Envelope{
			InstanceId: "42",
			Tags: map[string]string{
				"index":      "vm-guid-123",
				"job":        "diego-cell",
				"az":         "z1",
				"deployment": "cf-prod",
				"address":    "10.0.0.1",
				"ip":         "10.0.0.2",
			},
		}

		meta := ExtractMetadataFromEnvelope(envelope)

		Expect(meta.ID).To(Equal("vm-guid-123"))
		Expect(meta.Job).To(Equal("diego-cell"))
		Expect(meta.Index).To(Equal("42"))
		Expect(meta.AZ).To(Equal("z1"))
		Expect(meta.Deployment).To(Equal("cf-prod"))
		Expect(meta.Address).To(Equal("10.0.0.1"))
		Expect(meta.IP).To(Equal("10.0.0.2"))
	})

	It("handles missing tags gracefully", func() {
		envelope := &loggregator_v2.Envelope{
			Tags: map[string]string{
				"job": "router",
			},
		}

		meta := ExtractMetadataFromEnvelope(envelope)

		Expect(meta.ID).To(Equal(""))
		Expect(meta.Job).To(Equal("router"))
		Expect(meta.Deployment).To(Equal(""))
	})
})


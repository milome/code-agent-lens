package main

import (
	"strings"
	"testing"
)

func TestEmbeddedChangelogDoesNotReferenceUpstreamBranding(t *testing.T) {
	for name, body := range map[string]string{
		"CHANGELOG.json":    string(changelogEN),
		"CHANGELOG_CN.json": string(changelogZH),
	} {
		for _, forbidden := range []string{"lich0821", "ccNexus", "upstream", "上游"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s must not expose upstream branding or ambiguous upstream wording: found %q", name, forbidden)
			}
		}
	}
}

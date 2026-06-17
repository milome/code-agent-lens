package updater

import (
	"strings"
	"testing"
)

func TestGitHubReleaseFeedUsesCodeAgentLensFork(t *testing.T) {
	const want = "https://api.github.com/repos/milome/code-agent-lens/releases/latest"

	if githubAPIURL != want {
		t.Fatalf("githubAPIURL = %q, want %q", githubAPIURL, want)
	}
	if strings.Contains(githubAPIURL, "lich0821/ccNexus") {
		t.Fatalf("githubAPIURL must not point to upstream ccNexus release feed: %q", githubAPIURL)
	}
}

func TestSanitizeReleaseChangelogHidesInheritedUpstreamWording(t *testing.T) {
	got := sanitizeReleaseChangelog("Changes inherited from Upstrem ccNexus by lich0821")

	for _, forbidden := range []string{"lich0821", "ccNexus", "upstream", "Upstrem", "上游"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized changelog still contains %q: %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "CodeAgentLens") {
		t.Fatalf("sanitized changelog should mention CodeAgentLens, got %q", got)
	}
}

func TestSanitizeReleaseChangelogKeepsCodeAgentLensNotes(t *testing.T) {
	const body = "Improved CodeAgentLens desktop startup behavior."

	if got := sanitizeReleaseChangelog(body); got != body {
		t.Fatalf("sanitizeReleaseChangelog(%q) = %q", body, got)
	}
}

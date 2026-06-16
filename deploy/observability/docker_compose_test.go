package observability

import (
	"os"
	"strings"
	"testing"
)

const sharedDevToolsDataMount = `"D:/DevTools/code-agent-lens/data:/data"`

func TestFullComposeUsesSharedDevToolsDataBindMount(t *testing.T) {
	body := readTextFile(t, "docker-compose.full.yaml")

	assertSharedDataBindMount(t, body)
	for _, forbidden := range []string{
		"code-agent-lens-data:/data",
		"code-agent-lens-observability:/data/observability",
		"code-agent-lens-data:",
		"code-agent-lens-observability:",
		"D:/DevTools/shared/ccnexus",
		"D:\\DevTools\\shared\\ccnexus",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("docker-compose.full.yaml must not use named app data volume %q", forbidden)
		}
	}
}

func TestServerComposeUsesSharedDevToolsDataBindMount(t *testing.T) {
	body := readTextFile(t, "../../cmd/server/docker-compose.yml")

	assertSharedDataBindMount(t, body)
	if strings.Contains(body, "./code-agent-lens/:/data") {
		t.Fatalf("cmd/server/docker-compose.yml must not use repo-local app data bind mount")
	}
	if strings.Contains(body, "../../.tmp/docker-observability:/data/observability") {
		t.Fatalf("cmd/server/docker-compose.yml must not split observability dumps into .tmp")
	}
	for _, forbidden := range []string{
		"D:/DevTools/shared/ccnexus",
		"D:\\DevTools\\shared\\ccnexus",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("cmd/server/docker-compose.yml must not use legacy ccNexus path %q", forbidden)
		}
	}
}

func assertSharedDataBindMount(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		sharedDevToolsDataMount,
		"CODE_AGENT_LENS_DATA_DIR",
		"/data",
		"CODE_AGENT_LENS_DB_PATH",
		"/data/code-agent-lens.db",
		"CODE_AGENT_LENS_OBS_DUMP_DIR",
		"/data/observability",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("compose file missing %q:\n%s", want, body)
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

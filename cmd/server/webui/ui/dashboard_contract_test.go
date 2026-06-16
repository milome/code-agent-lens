package ui_test

import (
	"os"
	"strings"
	"testing"
)

func TestDashboardLoadsCurrentEndpointWithoutEmbeddingLogs(t *testing.T) {
	source, err := os.ReadFile("js/components/dashboard.js")
	if err != nil {
		t.Fatalf("failed to read dashboard component: %v", err)
	}
	dashboard := string(source)

	for _, expected := range []string{
		"api.getCurrentEndpoint()",
		"updateCurrentEndpoint",
	} {
		if !strings.Contains(dashboard, expected) {
			t.Fatalf("dashboard.js must include %q so migrated active endpoint is visible on 3010", expected)
		}
	}

	for _, forbidden := range []string{
		"api.getLogs(",
		"updateRecentLogs",
		"recent-logs",
		"dashboard.recentLogs",
	} {
		if strings.Contains(dashboard, forbidden) {
			t.Fatalf("dashboard.js must not include %q; logs belong on the left-nav Logs page", forbidden)
		}
	}
}

func TestLogsPageIsRegisteredBelowTesting(t *testing.T) {
	mainSource, err := os.ReadFile("js/main.js")
	if err != nil {
		t.Fatalf("failed to read main.js: %v", err)
	}
	mainJS := string(mainSource)
	for _, expected := range []string{
		"import { logs } from './components/logs.js';",
		"router.register('logs', logs);",
	} {
		if !strings.Contains(mainJS, expected) {
			t.Fatalf("main.js must include %q", expected)
		}
	}

	indexSource, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	indexHTML := string(indexSource)
	testingNav := strings.Index(indexHTML, `data-view="testing"`)
	logsNav := strings.Index(indexHTML, `data-view="logs"`)
	if testingNav < 0 || logsNav < 0 {
		t.Fatalf("index.html must include testing and logs nav entries")
	}
	if logsNav < testingNav {
		t.Fatalf("logs nav entry must be below testing nav entry")
	}

	logsSource, err := os.ReadFile("js/components/logs.js")
	if err != nil {
		t.Fatalf("failed to read logs component: %v", err)
	}
	logsJS := string(logsSource)
	for _, expected := range []string{
		"api.getLogs({ level: this.level, limit: this.limit })",
		"api.clearLogs()",
		"logs-auto-refresh",
	} {
		if !strings.Contains(logsJS, expected) {
			t.Fatalf("logs.js must include %q", expected)
		}
	}
}

func TestEndpointModalSupportsTokenPoolAuthModes(t *testing.T) {
	source, err := os.ReadFile("js/components/endpoints.js")
	if err != nil {
		t.Fatalf("failed to read endpoints component: %v", err)
	}
	endpointsJS := string(source)

	for _, expected := range []string{
		`name="authMode"`,
		`value="api_key"`,
		`value="token_pool"`,
		`value="codex_token_pool"`,
		`https://chatgpt.com/backend-api/codex`,
		`CODEX_TOKEN_POOL_TRANSFORMER`,
		`authMode: formData.get('authMode')`,
		`isTokenPoolAuthMode`,
	} {
		if !strings.Contains(endpointsJS, expected) {
			t.Fatalf("endpoints.js must include %q so new endpoints can be created with token pool auth modes", expected)
		}
	}
}

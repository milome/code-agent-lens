package viewer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/milome/code-agent-lens/internal/observability/dump"
)

func TestRegisterDisabledDoesNotExposeRoutes(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), false)
	handler := Guard(mux)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/obs/trace/trace-1", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestViewerShowsTraceOverviewPromptBodyAndStream(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixture(t, root)

	mux := http.NewServeMux()
	Register(mux, root, true, func() RuntimeSnapshot {
		return RuntimeSnapshot{
			Status:          "healthy",
			Mode:            "docker_compose",
			GatewayListener: "http://127.0.0.1:3010",
			PortalListener:  "http://127.0.0.1:3011/debug/obs",
			DumpRoot:        root,
			UpdatedAt:       "2026-06-17T00:00:00Z",
		}
	})
	handler := Guard(mux)

	traceResp := request(handler, "/debug/obs/trace/"+req.TraceID)
	if traceResp.Code != http.StatusOK {
		t.Fatalf("trace status = %d body=%s", traceResp.Code, traceResp.Body.String())
	}
	traceBody := traceResp.Body.String()
	for _, want := range []string{req.RequestID, "obs_ref", "system prompt", "raw upstream body", "/debug/obs", "/debug/obs/prompts"} {
		if !strings.Contains(traceBody, want) {
			t.Fatalf("trace body missing %q: %s", want, traceBody)
		}
	}

	systemResp := request(handler, "/debug/obs/request/"+req.RequestID+"/prompt/system")
	if got := strings.TrimSpace(systemResp.Body.String()); systemResp.Code != http.StatusOK || got != "system prompt" {
		t.Fatalf("system response status=%d body=%q", systemResp.Code, got)
	}

	bodyResp := request(handler, "/debug/obs/request/"+req.RequestID+"/body/upstream.response.body.raw")
	if got := strings.TrimSpace(bodyResp.Body.String()); bodyResp.Code != http.StatusOK || got != "raw upstream body" {
		t.Fatalf("body response status=%d body=%q", bodyResp.Code, got)
	}

	streamResp := request(handler, "/debug/obs/request/"+req.RequestID+"/stream/raw")
	if streamResp.Code != http.StatusOK || !strings.Contains(streamResp.Body.String(), "raw sse event") {
		t.Fatalf("stream response status=%d body=%q", streamResp.Code, streamResp.Body.String())
	}
}

func TestViewerShowsAllPromptsPage(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixture(t, root)

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs/request/"+req.RequestID+"/prompts")
	if resp.Code != http.StatusOK {
		t.Fatalf("prompts status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{
		"All Prompts",
		"request_id: request-1",
		"/debug/obs",
		"system",
		"system prompt",
		"developer",
		"developer prompt",
		"user",
		"user prompt",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("prompts body missing %q: %s", want, body)
		}
	}
	if !(strings.Index(body, "system") < strings.Index(body, "developer") && strings.Index(body, "developer") < strings.Index(body, "user")) {
		t.Fatalf("prompts not rendered in manifest order: %s", body)
	}
}

func TestViewerShowsPortalWithToolLinks(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixture(t, root)

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs")
	if resp.Code != http.StatusOK {
		t.Fatalf("portal status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{
		"Debug Portal",
		"Prompt Sessions",
		"LLM API Errors",
		"Filtered Noise",
		"Raw Total",
		"Primary Actions",
		"Docker Observability Stack",
		"Debug Viewer",
		"Prompt Explorer",
		"/debug/obs/prompts",
		"/debug/obs/tool/jaeger",
		"/debug/obs/tool/grafana",
		"Jaeger",
		"http://127.0.0.1:16686",
		"Grafana",
		"http://127.0.0.1:13000",
		"Prometheus",
		"http://127.0.0.1:9090/graph",
		"Tempo",
		"http://127.0.0.1:3200/status",
		"OTel Collector",
		"http://127.0.0.1:8888/metrics",
		"http://127.0.0.1:3010/health",
		"http://127.0.0.1:3010/ui/",
		req.RequestID,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("portal body missing %q: %s", want, body)
		}
	}
	for _, href := range []string{
		"http://127.0.0.1:16686",
		"http://127.0.0.1:13000",
		"http://127.0.0.1:9090/graph",
		"http://127.0.0.1:3200/status",
		"http://127.0.0.1:8888/metrics",
		"http://127.0.0.1:3010/health",
		"http://127.0.0.1:3010/ui/",
	} {
		want := `href="` + href + `" target="_blank" rel="noopener noreferrer"`
		if !strings.Contains(body, want) {
			t.Fatalf("portal external link missing attrs %q: %s", want, body)
		}
	}
	for _, href := range []string{"/debug/obs/prompts", "/debug/obs/tool/jaeger", "/debug/obs/tool/grafana"} {
		if strings.Contains(body, `href="`+href+`" target="_blank"`) {
			t.Fatalf("portal internal link should stay in same tab: %s", body)
		}
	}
	assertPortalLinksUseExpectedPorts(t, body)
}

func TestViewerShowsRuntimeStatusAndActiveEndpoint(t *testing.T) {
	root := t.TempDir()

	mux := http.NewServeMux()
	Register(mux, root, true, func() RuntimeSnapshot {
		return RuntimeSnapshot{
			Status:           "healthy",
			Mode:             "docker_compose",
			GatewayListener:  "http://127.0.0.1:3010",
			PortalListener:   "http://127.0.0.1:3011/debug/obs",
			DumpRoot:         root,
			TotalEndpoints:   2,
			EnabledEndpoints: 1,
			ActiveEndpoint: &EndpointSnapshot{
				Name:             "Rightcode",
				AuthMode:         "api_key",
				Enabled:          true,
				Current:          true,
				Transformer:      "openai2",
				Model:            "gpt-5.5",
				BaseURLHost:      "right.codes",
				Health:           "active",
				LastSwitchReason: "current proxy index",
			},
			Endpoints: []EndpointSnapshot{
				{
					Name:        "Rightcode",
					Enabled:     true,
					Current:     true,
					Transformer: "openai2",
					Model:       "gpt-5.5",
					BaseURLHost: "right.codes",
					Health:      "active",
				},
				{
					Name:        "Disabled",
					Enabled:     false,
					Transformer: "claude",
					BaseURLHost: "example.invalid",
					Health:      "disabled",
				},
			},
			UpdatedAt: "2026-06-16T12:00:00Z",
		}
	})
	handler := Guard(mux)

	portalResp := request(handler, "/debug/obs")
	if portalResp.Code != http.StatusOK {
		t.Fatalf("portal status=%d body=%s", portalResp.Code, portalResp.Body.String())
	}
	portalBody := portalResp.Body.String()
	for _, want := range []string{"Runtime", "Active Endpoint", "Rightcode", "/debug/obs/runtime"} {
		if !strings.Contains(portalBody, want) {
			t.Fatalf("portal missing %q: %s", want, portalBody)
		}
	}

	pageResp := request(handler, "/debug/obs/runtime")
	if pageResp.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", pageResp.Code, pageResp.Body.String())
	}
	pageBody := pageResp.Body.String()
	for _, want := range []string{
		"Runtime Status",
		"Active Endpoint",
		"Current Endpoint",
		"Rightcode",
		"right.codes",
		"docker_compose",
		"http://127.0.0.1:3011/debug/obs",
	} {
		if !strings.Contains(pageBody, want) {
			t.Fatalf("runtime page missing %q: %s", want, pageBody)
		}
	}
	if strings.Contains(pageBody, "apiKey") || strings.Contains(pageBody, "sk-") {
		t.Fatalf("runtime page must not expose secrets: %s", pageBody)
	}
	assertPortalLinksUseExpectedPorts(t, pageBody)

	statusResp := request(handler, "/debug/obs/api/runtime/status")
	if statusResp.Code != http.StatusOK {
		t.Fatalf("runtime api status=%d body=%s", statusResp.Code, statusResp.Body.String())
	}
	statusBody := statusResp.Body.String()
	for _, want := range []string{`"mode":"docker_compose"`, `"enabledEndpoints":1`, `"baseUrlHost":"right.codes"`} {
		if !strings.Contains(statusBody, want) {
			t.Fatalf("runtime api missing %q: %s", want, statusBody)
		}
	}
	if strings.Contains(statusBody, "apiKey") || strings.Contains(statusBody, "sk-") {
		t.Fatalf("runtime api must not expose secrets: %s", statusBody)
	}

	activeResp := request(handler, "/debug/obs/api/endpoints/active")
	if activeResp.Code != http.StatusOK {
		t.Fatalf("active api status=%d body=%s", activeResp.Code, activeResp.Body.String())
	}
	activeBody := activeResp.Body.String()
	for _, want := range []string{`"activeEndpoint"`, `"name":"Rightcode"`, `"baseUrlHost":"right.codes"`} {
		if !strings.Contains(activeBody, want) {
			t.Fatalf("active api missing %q: %s", want, activeBody)
		}
	}
}

func TestViewerPagesKeepGatewayAndPortalLinksOnSeparatePorts(t *testing.T) {
	root := t.TempDir()
	createViewerFixture(t, root)
	mux := http.NewServeMux()
	Register(mux, root, true, func() RuntimeSnapshot {
		return RuntimeSnapshot{
			Status:           "healthy",
			Mode:             "docker_compose",
			GatewayListener:  "http://127.0.0.1:3010",
			PortalListener:   "http://127.0.0.1:3011/debug/obs",
			DumpRoot:         root,
			TotalEndpoints:   1,
			EnabledEndpoints: 1,
			UpdatedAt:        "2026-06-17T00:00:00Z",
		}
	})
	handler := Guard(mux)

	for _, path := range []string{
		"/debug/obs",
		"/debug/obs/runtime",
		"/debug/obs/prompts",
		"/debug/obs/tool/jaeger",
		"/debug/obs/tool/grafana",
	} {
		resp := request(handler, path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, resp.Code, resp.Body.String())
		}
		assertPortalLinksUseExpectedPorts(t, resp.Body.String())
	}
}

func TestViewerPortalPaginatesAllSessionsNewestFirst(t *testing.T) {
	root := t.TempDir()
	oldPrompt := createViewerFixtureWithIDs(t, root, "run-sessions", "trace-old-prompt", "span-1", "request-z-old-prompt", "old")
	time.Sleep(2 * time.Millisecond)
	noPrompt := createViewerFixtureWithoutPrompts(t, root, "run-sessions", "trace-no-prompt", "span-2", "request-y-no-prompt")
	time.Sleep(2 * time.Millisecond)
	midPrompt := createViewerFixtureWithIDs(t, root, "run-sessions", "trace-mid-prompt", "span-3", "request-m-mid-prompt", "mid")
	time.Sleep(2 * time.Millisecond)
	newPrompt := createViewerFixtureWithIDs(t, root, "run-sessions", "trace-new-prompt", "span-4", "request-a-new-prompt", "new")

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	firstPage := request(handler, "/debug/obs?limit=2")
	if firstPage.Code != http.StatusOK {
		t.Fatalf("portal status = %d body=%s", firstPage.Code, firstPage.Body.String())
	}
	firstBody := firstPage.Body.String()
	for _, want := range []string{"Prompt Sessions", newPrompt.RequestID, midPrompt.RequestID, "Next"} {
		if !strings.Contains(firstBody, want) {
			t.Fatalf("portal first page missing %q: %s", want, firstBody)
		}
	}
	if strings.Contains(firstBody, oldPrompt.RequestID) || strings.Contains(firstBody, noPrompt.RequestID) {
		t.Fatalf("portal first page should only show newest useful page rows: %s", firstBody)
	}
	if !(strings.Index(firstBody, newPrompt.RequestID) < strings.Index(firstBody, midPrompt.RequestID)) {
		t.Fatalf("portal first page should be newest first: %s", firstBody)
	}

	secondPage := request(handler, "/debug/obs?limit=2&offset=2")
	if secondPage.Code != http.StatusOK {
		t.Fatalf("portal second page status = %d body=%s", secondPage.Code, secondPage.Body.String())
	}
	secondBody := secondPage.Body.String()
	for _, want := range []string{oldPrompt.RequestID, "Previous"} {
		if !strings.Contains(secondBody, want) {
			t.Fatalf("portal second page missing %q: %s", want, secondBody)
		}
	}
	if strings.Contains(secondBody, noPrompt.RequestID) || strings.Contains(secondBody, newPrompt.RequestID) || strings.Contains(secondBody, midPrompt.RequestID) {
		t.Fatalf("portal second page should not show first page rows: %s", secondBody)
	}

	noisePage := request(handler, "/debug/obs?view=noise")
	if noisePage.Code != http.StatusOK {
		t.Fatalf("portal noise page status = %d body=%s", noisePage.Code, noisePage.Body.String())
	}
	noiseBody := noisePage.Body.String()
	if !strings.Contains(noiseBody, noPrompt.RequestID) {
		t.Fatalf("portal noise page should include prompt-less noise row: %s", noiseBody)
	}
}

func TestViewerPortalFiltersPromptErrorsAllAndNoiseSessions(t *testing.T) {
	root := t.TempDir()
	olderPrompt := createViewerFixtureWithIDs(t, root, "run-filtered", "trace-older-prompt", "span-1", "request-older-prompt", "alpha")
	time.Sleep(2 * time.Millisecond)
	jsonReq := createViewerFixtureWithoutPromptsWithIngress(t, root, "run-filtered", "trace-json", "span-2", "request-json", map[string][]string{
		"Accept":       {"application/json"},
		"Content-Type": {"application/json"},
	}, []byte(`{"model":"gpt-test","messages":[]}`))
	time.Sleep(2 * time.Millisecond)
	newerPrompt := createViewerFixtureWithIDs(t, root, "run-filtered", "trace-newer-prompt", "span-3", "request-newer-prompt", "beta")
	time.Sleep(2 * time.Millisecond)
	noiseReq := createViewerFixtureWithoutPromptsWithIngress(t, root, "run-filtered", "trace-noise", "span-3", "request-noise", map[string][]string{
		"Accept":         {"image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"},
		"Sec-Fetch-Dest": {"image"},
		"Referer":        {"http://127.0.0.1:3011/debug/obs"},
	}, nil)

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	defaultResp := request(handler, "/debug/obs?limit=10")
	if defaultResp.Code != http.StatusOK {
		t.Fatalf("portal status = %d body=%s", defaultResp.Code, defaultResp.Body.String())
	}
	defaultBody := defaultResp.Body.String()
	for _, want := range []string{"Prompt Sessions", "LLM API Errors", "All LLM/API", "Filtered Noise", newerPrompt.RequestID, olderPrompt.RequestID, "Sort"} {
		if !strings.Contains(defaultBody, want) {
			t.Fatalf("default portal missing %q: %s", want, defaultBody)
		}
	}
	if strings.Contains(defaultBody, jsonReq.RequestID) || strings.Contains(defaultBody, noiseReq.RequestID) {
		t.Fatalf("default portal should show only prompt sessions: %s", defaultBody)
	}
	if !(strings.Index(defaultBody, newerPrompt.RequestID) < strings.Index(defaultBody, olderPrompt.RequestID)) {
		t.Fatalf("default portal should sort prompt sessions newest first: %s", defaultBody)
	}

	oldestResp := request(handler, "/debug/obs?sort=oldest&limit=10")
	oldestBody := oldestResp.Body.String()
	if !(strings.Index(oldestBody, olderPrompt.RequestID) < strings.Index(oldestBody, newerPrompt.RequestID)) {
		t.Fatalf("sort=oldest should render oldest prompt first: %s", oldestBody)
	}

	searchResp := request(handler, "/debug/obs?q=newer&limit=10")
	searchBody := searchResp.Body.String()
	if !strings.Contains(searchBody, newerPrompt.RequestID) || strings.Contains(searchBody, olderPrompt.RequestID) {
		t.Fatalf("q filter should narrow prompt sessions by request/trace/run: %s", searchBody)
	}

	roleResp := request(handler, "/debug/obs?role=developer&limit=10")
	roleBody := roleResp.Body.String()
	if !strings.Contains(roleBody, newerPrompt.RequestID) || !strings.Contains(roleBody, olderPrompt.RequestID) {
		t.Fatalf("role filter should keep sessions with developer prompts: %s", roleBody)
	}

	errorsResp := request(handler, "/debug/obs?view=errors&limit=10")
	if errorsResp.Code != http.StatusOK {
		t.Fatalf("errors portal status = %d body=%s", errorsResp.Code, errorsResp.Body.String())
	}
	errorsBody := errorsResp.Body.String()
	for _, want := range []string{"LLM API Errors", jsonReq.RequestID} {
		if !strings.Contains(errorsBody, want) {
			t.Fatalf("errors portal missing %q: %s", want, errorsBody)
		}
	}
	if strings.Contains(errorsBody, newerPrompt.RequestID) || strings.Contains(errorsBody, olderPrompt.RequestID) || strings.Contains(errorsBody, noiseReq.RequestID) {
		t.Fatalf("errors portal should only show LLM/API no-prompt sessions: %s", errorsBody)
	}

	allResp := request(handler, "/debug/obs?view=all&limit=10")
	allBody := allResp.Body.String()
	for _, want := range []string{"All LLM/API", newerPrompt.RequestID, olderPrompt.RequestID, jsonReq.RequestID} {
		if !strings.Contains(allBody, want) {
			t.Fatalf("all portal missing %q: %s", want, allBody)
		}
	}
	if strings.Contains(allBody, noiseReq.RequestID) {
		t.Fatalf("all portal should still hide noise: %s", allBody)
	}

	noiseResp := request(handler, "/debug/obs?view=noise&limit=10")
	if noiseResp.Code != http.StatusOK {
		t.Fatalf("noise portal status = %d body=%s", noiseResp.Code, noiseResp.Body.String())
	}
	noiseBody := noiseResp.Body.String()
	for _, want := range []string{"Filtered Noise", noiseReq.RequestID, "Prompt Sessions"} {
		if !strings.Contains(noiseBody, want) {
			t.Fatalf("noise portal missing %q: %s", want, noiseBody)
		}
	}
	if strings.Contains(noiseBody, newerPrompt.RequestID) || strings.Contains(noiseBody, olderPrompt.RequestID) || strings.Contains(noiseBody, jsonReq.RequestID) {
		t.Fatalf("noise portal should not show LLM/API sessions: %s", noiseBody)
	}
}

func TestViewerShowsGlobalPromptsPage(t *testing.T) {
	root := t.TempDir()
	older := createViewerFixtureWithIDs(t, root, "run-1", "trace-1", "span-1", "request-1", "older")
	newer := createViewerFixtureWithIDs(t, root, "run-2", "trace-2", "span-2", "request-2", "newer")
	empty := createViewerFixtureWithoutPrompts(t, root, "run-3", "trace-3", "span-3", "request-3")
	long := createViewerFixtureWithLongPrompt(t, root, "run-4", "trace-4", "span-4", "request-4")

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs/prompts?limit=10")
	if resp.Code != http.StatusOK {
		t.Fatalf("global prompts status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{
		"Prompt Explorer",
		"Filters",
		"Prompt Requests",
		"Trace ID",
		"hover-card",
		"trace-list",
		long.RequestID,
		newer.RequestID,
		older.RequestID,
		"/debug/obs/request/" + newer.RequestID + "/prompt-preview",
		"/debug/obs/request/" + newer.RequestID + "/prompts",
		"/debug/obs/trace/" + newer.TraceID,
		"role-badge",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("global prompts body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, long.UniqueTail) {
		t.Fatalf("global prompts should not render full long prompt body: %s", body)
	}
	if !(strings.Index(body, long.RequestID) < strings.Index(body, newer.RequestID) && strings.Index(body, newer.RequestID) < strings.Index(body, older.RequestID)) {
		t.Fatalf("global prompts not rendered newest first: %s", body)
	}
	if strings.Contains(body, empty.RequestID) {
		t.Fatalf("global prompts should skip requests without prompts: %s", body)
	}
	if strings.Contains(body, "request-card prompt-summary") {
		t.Fatalf("global prompts should render dense trace rows instead of summary cards: %s", body)
	}
	if strings.Contains(body, "newer system prompt") || strings.Contains(body, "older system prompt") {
		t.Fatalf("global prompts should lazy-load prompt previews instead of rendering snippets inline: %s", body)
	}

	previewResp := request(handler, "/debug/obs/request/"+newer.RequestID+"/prompt-preview")
	if previewResp.Code != http.StatusOK {
		t.Fatalf("prompt preview status = %d body=%s", previewResp.Code, previewResp.Body.String())
	}
	previewBody := previewResp.Body.String()
	for _, want := range []string{"newer system", "newer developer", "newer user", "compact-snippet"} {
		if !strings.Contains(previewBody, want) {
			t.Fatalf("prompt preview body missing %q: %s", want, previewBody)
		}
	}
}

func TestViewerFiltersAndPaginatesGlobalPrompts(t *testing.T) {
	root := t.TempDir()
	_ = createViewerFixtureWithIDs(t, root, "run-1", "trace-alpha", "span-1", "request-alpha", "alpha")
	beta := createViewerFixtureWithIDs(t, root, "run-2", "trace-beta", "span-2", "request-beta", "beta")
	gamma := createViewerFixtureWithIDs(t, root, "run-3", "trace-gamma", "span-3", "request-gamma", "gamma")

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	searchResp := request(handler, "/debug/obs/prompts?q=beta&role=developer&run=run-2")
	if searchResp.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", searchResp.Code, searchResp.Body.String())
	}
	searchBody := searchResp.Body.String()
	if !strings.Contains(searchBody, beta.RequestID) || strings.Contains(searchBody, gamma.RequestID) {
		t.Fatalf("search did not filter correctly: %s", searchBody)
	}
	if !strings.Contains(searchBody, `name="q" value="beta"`) || !strings.Contains(searchBody, `name="role" value="developer"`) || !strings.Contains(searchBody, `name="run" value="run-2"`) {
		t.Fatalf("search form missing query values: %s", searchBody)
	}

	pageResp := request(handler, "/debug/obs/prompts?limit=1&offset=1")
	if pageResp.Code != http.StatusOK {
		t.Fatalf("page status = %d body=%s", pageResp.Code, pageResp.Body.String())
	}
	pageBody := pageResp.Body.String()
	if !strings.Contains(pageBody, beta.RequestID) || strings.Contains(pageBody, gamma.RequestID) {
		t.Fatalf("pagination did not select second newest request: %s", pageBody)
	}
	if !strings.Contains(pageBody, "Previous") || !strings.Contains(pageBody, "Next") {
		t.Fatalf("pagination links missing: %s", pageBody)
	}
}

func TestViewerGlobalPromptsSortsNewestRequestTimestampFirst(t *testing.T) {
	root := t.TempDir()
	older := createViewerFixtureWithIDs(t, root, "run-same", "trace-old", "span-old", "request-z-old", "older")
	time.Sleep(2 * time.Millisecond)
	newer := createViewerFixtureWithIDs(t, root, "run-same", "trace-new", "span-new", "request-a-new", "newer")

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs/prompts?limit=10")
	if resp.Code != http.StatusOK {
		t.Fatalf("global prompts status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !(strings.Index(body, newer.RequestID) < strings.Index(body, older.RequestID)) {
		t.Fatalf("global prompts should render newest timestamp first, body=%s", body)
	}
}

func TestViewerGlobalPromptsOnlyReadsCurrentPagePreviews(t *testing.T) {
	root := t.TempDir()
	first := createViewerFixtureWithIDs(t, root, "run-3", "trace-first", "span-1", "request-first", "first")
	second := createViewerFixtureWithIDs(t, root, "run-2", "trace-second", "span-2", "request-second", "second")
	third := createViewerFixtureWithIDs(t, root, "run-1", "trace-third", "span-3", "request-third", "third")
	missingPreviewPath := filepath.Join(root, "runs", "run-2", "traces", second.TraceID, second.RequestID, "prompt.system.txt")
	if err := os.Remove(missingPreviewPath); err != nil {
		t.Fatalf("remove off-page prompt file: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs/prompts?limit=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("global prompts status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, third.RequestID) {
		t.Fatalf("first page missing newest request: %s", body)
	}
	if strings.Contains(body, first.RequestID) || strings.Contains(body, second.RequestID) {
		t.Fatalf("first page should only render current page rows: %s", body)
	}
	if strings.Contains(body, "unable to read prompt file") {
		t.Fatalf("off-page prompt preview should not be read: %s", body)
	}
}

func TestViewerHandlesRequestWithoutPrompts(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixtureWithoutPrompts(t, root, "run-1", "trace-1", "span-1", "request-1")

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	promptsResp := request(handler, "/debug/obs/request/"+req.RequestID+"/prompts")
	if promptsResp.Code != http.StatusOK {
		t.Fatalf("prompts status = %d body=%s", promptsResp.Code, promptsResp.Body.String())
	}
	promptsBody := promptsResp.Body.String()
	if !strings.Contains(promptsBody, "No prompts captured for this request") {
		t.Fatalf("empty prompts page missing explanation: %s", promptsBody)
	}

	portalResp := request(handler, "/debug/obs")
	portalBody := portalResp.Body.String()
	if strings.Contains(portalBody, req.RequestID) {
		t.Fatalf("default portal should hide prompt-less noise request: %s", portalBody)
	}

	noiseResp := request(handler, "/debug/obs?view=noise")
	noiseBody := noiseResp.Body.String()
	if !strings.Contains(noiseBody, req.RequestID) {
		t.Fatalf("noise portal should include prompt-less request: %s", noiseBody)
	}
	if strings.Contains(noiseBody, "/debug/obs/request/"+req.RequestID+"/prompts") {
		t.Fatalf("noise portal should not show prompts action for prompt-less request: %s", noiseBody)
	}
}

func TestViewerRejectsUnsafeAndUnmanifestedFiles(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixture(t, root)
	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	paths := []string{
		"/debug/obs/request/" + req.RequestID + "/body/../prompt.system.txt",
		"/debug/obs/request/" + req.RequestID + "/body/C:/secret.txt",
		"/debug/obs/request/" + req.RequestID + "/body/not-in-manifest.txt",
	}
	for _, path := range paths {
		resp := request(handler, path)
		if resp.Code != http.StatusBadRequest && resp.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 400 or 404", path, resp.Code)
		}
	}
}

func TestViewerShowsArtifactViewerAndPreservesRawEndpoints(t *testing.T) {
	root := t.TempDir()
	req := createViewerFixture(t, root)
	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	artifactResp := request(handler, "/debug/obs/request/"+req.RequestID+"/artifact/upstream.response.body.raw")
	if artifactResp.Code != http.StatusOK {
		t.Fatalf("artifact status=%d body=%s", artifactResp.Code, artifactResp.Body.String())
	}
	artifactBody := artifactResp.Body.String()
	for _, want := range []string{
		"Artifact Viewer",
		"/debug/obs",
		"raw upstream body",
		"/debug/obs/request/" + req.RequestID + "/body/upstream.response.body.raw",
	} {
		if !strings.Contains(artifactBody, want) {
			t.Fatalf("artifact body missing %q: %s", want, artifactBody)
		}
	}

	rawResp := request(handler, "/debug/obs/request/"+req.RequestID+"/body/upstream.response.body.raw")
	if got := strings.TrimSpace(rawResp.Body.String()); rawResp.Code != http.StatusOK || got != "raw upstream body" {
		t.Fatalf("raw response status=%d body=%q", rawResp.Code, got)
	}

	for _, path := range []string{
		"/debug/obs/request/" + req.RequestID + "/artifact/../prompt.system.txt",
		"/debug/obs/request/" + req.RequestID + "/artifact/C:/secret.txt",
		"/debug/obs/request/" + req.RequestID + "/artifact/not-in-manifest.txt",
	} {
		resp := request(handler, path)
		if resp.Code != http.StatusBadRequest && resp.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 400 or 404", path, resp.Code)
		}
	}
}

func TestViewerShowsToolWrappers(t *testing.T) {
	root := t.TempDir()
	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	tests := []struct {
		path       string
		title      string
		breadcrumb string
		iframe     string
		native     string
	}{
		{"/debug/obs/tool/jaeger", "Jaeger", "Portal / Tools / Jaeger", "http://127.0.0.1:16686", "Open Jaeger native"},
		{"/debug/obs/tool/grafana", "Grafana", "Portal / Tools / Grafana", "http://127.0.0.1:13000", "Open Grafana native"},
	}
	for _, tt := range tests {
		resp := request(handler, tt.path)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tt.path, resp.Code, resp.Body.String())
		}
		body := resp.Body.String()
		for _, want := range []string{tt.title, tt.breadcrumb, `src="` + tt.iframe + `"`, tt.native, `href="` + tt.iframe + `" target="_blank" rel="noopener noreferrer"`, "frame-policy", "/debug/obs"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q: %s", tt.path, want, body)
			}
		}
	}
}

func TestViewerShowsCodexToolCallChain(t *testing.T) {
	root := t.TempDir()
	sessionsRoot := filepath.Join(root, "codex-sessions")
	t.Setenv("CODE_AGENT_LENS_CODEX_SESSIONS_ROOT", sessionsRoot)
	writeCodexRolloutFixture(t, sessionsRoot)

	mux := http.NewServeMux()
	Register(mux, root, true)
	handler := Guard(mux)

	resp := request(handler, "/debug/obs/codex/tool-calls")
	if resp.Code != http.StatusOK {
		t.Fatalf("tool calls status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{
		"Codex Tool Call Chain",
		"turn-1",
		"trace-abc123",
		"shell_command",
		"call-shell-1",
		"Get-ChildItem",
		"Exit code: 0",
		"http://127.0.0.1:16686/trace/trace-abc123",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("tool call chain body missing %q: %s", want, body)
		}
	}
}

type fixtureRequest struct {
	TraceID    string
	RequestID  string
	UniqueTail string
}

func createViewerFixture(t *testing.T, root string) fixtureRequest {
	t.Helper()
	return createViewerFixtureWithIDs(t, root, "run-1", "trace-1", "span-1", "request-1", "")
}

func createViewerFixtureWithIDs(t *testing.T, root, runID, traceID, spanID, requestID, prefix string) fixtureRequest {
	t.Helper()
	if prefix != "" {
		prefix += " "
	}
	w := dump.NewWriter(dump.Config{Enabled: true, Root: root, RunID: runID})
	rw, err := w.BeginRequest(traceID, spanID, requestID)
	if err != nil {
		t.Fatalf("BeginRequest: %v", err)
	}
	system, err := rw.WriteFile("prompt.system.txt", []byte(prefix+"system prompt"))
	if err != nil {
		t.Fatalf("write system: %v", err)
	}
	developer, err := rw.WriteFile("prompt.developer.txt", []byte(prefix+"developer prompt"))
	if err != nil {
		t.Fatalf("write developer: %v", err)
	}
	user, err := rw.WriteFile("prompt.user.txt", []byte(prefix+"user prompt"))
	if err != nil {
		t.Fatalf("write user: %v", err)
	}
	upstream, err := rw.WriteFile("upstream.response.body.raw", []byte("raw upstream body"))
	if err != nil {
		t.Fatalf("write upstream: %v", err)
	}
	stream, err := rw.AppendJSONL("stream.raw.events.jsonl", map[string]any{"data": "raw sse event"})
	if err != nil {
		t.Fatalf("write stream: %v", err)
	}
	if err := rw.WritePromptIndex(dump.PromptIndex{
		Prompts: []dump.PromptRecord{
			{Role: "system", File: "prompt.system.txt"},
			{Role: "developer", File: "prompt.developer.txt"},
			{Role: "user", File: "prompt.user.txt"},
		},
		Files: []dump.FileRecord{system, developer, user, upstream, stream},
	}); err != nil {
		t.Fatalf("write prompt index: %v", err)
	}
	// Force an unmanifested file into the request directory.
	if err := os.WriteFile(filepath.Join(root, "runs", runID, "traces", traceID, requestID, "not-in-manifest.txt"), []byte("secret"), 0600); err != nil {
		t.Fatalf("write unmanifested: %v", err)
	}
	return fixtureRequest{TraceID: traceID, RequestID: requestID}
}

func createViewerFixtureWithoutPrompts(t *testing.T, root, runID, traceID, spanID, requestID string) fixtureRequest {
	t.Helper()
	return createViewerFixtureWithoutPromptsWithIngress(t, root, runID, traceID, spanID, requestID, nil, nil)
}

func createViewerFixtureWithoutPromptsWithIngress(t *testing.T, root, runID, traceID, spanID, requestID string, headers map[string][]string, body []byte) fixtureRequest {
	t.Helper()
	w := dump.NewWriter(dump.Config{Enabled: true, Root: root, RunID: runID})
	rw, err := w.BeginRequest(traceID, spanID, requestID)
	if err != nil {
		t.Fatalf("BeginRequest: %v", err)
	}
	var files []dump.FileRecord
	if headers != nil {
		headerFile, err := rw.WriteJSONFile("ingress.request.headers.json", headers)
		if err != nil {
			t.Fatalf("write headers: %v", err)
		}
		files = append(files, headerFile)
	}
	bodyFile, err := rw.WriteFile("ingress.request.body.raw", body)
	if err != nil {
		t.Fatalf("write ingress body: %v", err)
	}
	files = append(files, bodyFile)
	if err := rw.WritePromptIndex(dump.PromptIndex{
		Prompts: nil,
		Files:   files,
	}); err != nil {
		t.Fatalf("write prompt index: %v", err)
	}
	return fixtureRequest{TraceID: traceID, RequestID: requestID}
}

func createViewerFixtureWithLongPrompt(t *testing.T, root, runID, traceID, spanID, requestID string) fixtureRequest {
	t.Helper()
	uniqueTail := "UNIQUE-LONG-PROMPT-TAIL-SHOULD-NOT-APPEAR"
	longText := strings.Repeat("long prompt segment ", 80) + uniqueTail
	w := dump.NewWriter(dump.Config{Enabled: true, Root: root, RunID: runID})
	rw, err := w.BeginRequest(traceID, spanID, requestID)
	if err != nil {
		t.Fatalf("BeginRequest: %v", err)
	}
	system, err := rw.WriteFile("prompt.system.txt", []byte(longText))
	if err != nil {
		t.Fatalf("write long system: %v", err)
	}
	if err := rw.WritePromptIndex(dump.PromptIndex{
		Prompts: []dump.PromptRecord{{Role: "system", File: "prompt.system.txt"}},
		Files:   []dump.FileRecord{system},
	}); err != nil {
		t.Fatalf("write prompt index: %v", err)
	}
	return fixtureRequest{TraceID: traceID, RequestID: requestID, UniqueTail: uniqueTail}
}

func request(handler http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func assertPortalLinksUseExpectedPorts(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, ":3021") {
		t.Fatalf("page must not link to legacy 3021 port: %s", body)
	}
	portalBase, err := url.Parse("http://127.0.0.1:3011/debug/obs")
	if err != nil {
		t.Fatalf("parse portal base: %v", err)
	}
	linkPattern := regexp.MustCompile(`(?i)(href|src|action)="([^"]+)"`)
	for _, match := range linkPattern.FindAllStringSubmatch(body, -1) {
		raw := strings.ReplaceAll(match[2], "&amp;", "&")
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse link %q: %v", raw, err)
		}
		resolved := portalBase.ResolveReference(parsed)
		switch resolved.Port() {
		case "3011":
			if !strings.HasPrefix(resolved.Path, "/debug/obs") {
				t.Fatalf("debug portal link points outside debug routes: raw=%q resolved=%s", raw, resolved.String())
			}
		case "3010":
			if strings.HasPrefix(resolved.Path, "/debug/obs") {
				t.Fatalf("gateway link points at debug route: raw=%q resolved=%s", raw, resolved.String())
			}
		}
	}
}

func writeCodexRolloutFixture(t *testing.T, sessionsRoot string) {
	t.Helper()
	dir := filepath.Join(sessionsRoot, "2026", "06", "13")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir codex sessions: %v", err)
	}
	lines := []string{
		`{"timestamp":"2026-06-13T06:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1","trace_id":"trace-abc123","started_at":1781330400}}`,
		`{"timestamp":"2026-06-13T06:00:01Z","type":"response_item","payload":{"type":"function_call","name":"shell_command","call_id":"call-shell-1","arguments":"{\"command\":\"Get-ChildItem\",\"workdir\":\"D:\\\\Dev\\\\CodeAgentLens\"}"}}`,
		`{"timestamp":"2026-06-13T06:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-shell-1","output":"Exit code: 0\nWall time: 0.1 seconds\nOutput:\nok"}}`,
	}
	path := filepath.Join(dir, "rollout-test.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatalf("write rollout fixture: %v", err)
	}
}

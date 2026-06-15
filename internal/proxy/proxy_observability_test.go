package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/observability"
)

func TestProxyRequestWritesLocalDebugDump(t *testing.T) {
	setProxyFakeOTLPEndpoint(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatalf("upstream authorization missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{{
		Name:        "OpenAI",
		APIUrl:      upstream.URL,
		APIKey:      "secret-key",
		AuthMode:    config.AuthModeAPIKey,
		Transformer: "openai",
		Model:       "gpt-test",
		Enabled:     true,
	}})
	cfg.UpdatePort(0)

	dumpDir := t.TempDir()
	rt, err := observability.Init(context.Background(), observability.Config{
		Enabled:             true,
		DumpEnabled:         true,
		DumpDir:             dumpDir,
		ViewerEnabled:       true,
		ViewerPublicURL:     "http://127.0.0.1:3011/debug/obs",
		CaptureHeaders:      "all",
		CaptureBodies:       "all",
		CaptureSecrets:      true,
		CaptureStreamEvents: "all",
		PromptExtract:       true,
		ServiceName:         "code-agent-lens-test",
	}, "test")
	if err != nil {
		t.Fatalf("init observability: %v", err)
	}
	defer rt.Shutdown(context.Background())

	p := New(cfg, nilStatsStorage{}, nil, "device")
	p.SetObservabilityRuntime(rt)

	body := `{"model":"client-model","messages":[{"role":"system","content":"system prompt"},{"role":"developer","content":"dev prompt"},{"role":"user","content":"user prompt"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.handleProxyRequest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	reqDir := latestRequestDir(t, dumpDir)
	for _, name := range []string{
		"ingress.request.headers.json",
		"ingress.request.body.raw",
		"prompt.index.json",
		"prompt.system.txt",
		"prompt.developer.txt",
		"prompt.user.txt",
		"transform.request.input.raw",
		"transform.request.output.raw",
		"upstream.request.url.json",
		"upstream.request.headers.json",
		"upstream.request.body.raw",
		"upstream.response.headers.json",
		"upstream.response.body.raw",
		"upstream.response.transformed.raw",
		"usage.json",
	} {
		if _, err := os.Stat(filepath.Join(reqDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	headerRaw, err := os.ReadFile(filepath.Join(reqDir, "upstream.request.headers.json"))
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	if !strings.Contains(string(headerRaw), "secret-key") {
		t.Fatalf("capture secrets enabled should retain api key: %s", headerRaw)
	}

	indexRaw, err := os.ReadFile(filepath.Join(reqDir, "prompt.index.json"))
	if err != nil {
		t.Fatalf("read prompt index: %v", err)
	}
	var index map[string]any
	if err := json.Unmarshal(indexRaw, &index); err != nil {
		t.Fatalf("prompt.index.json invalid: %v", err)
	}
	for _, key := range []string{"run_id", "request_id", "trace_id", "span_id", "prompts", "files"} {
		if _, ok := index[key]; !ok {
			t.Fatalf("prompt.index.json missing %s: %s", key, indexRaw)
		}
	}
}

func latestRequestDir(t *testing.T, root string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "runs", "*", "traces", "*", "*"))
	if err != nil {
		t.Fatalf("glob request dirs: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no request dump dirs under %s", root)
	}
	return matches[len(matches)-1]
}

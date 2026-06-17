package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/storage"
)

func TestInvalidJSONRequestDoesNotRotateCurrentEndpoint(t *testing.T) {
	var upstreamHits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamHits, 1)
		http.Error(w, "unexpected upstream call", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Endpoints: []config.Endpoint{
			{Name: "大白AI", APIUrl: upstream.URL + "/dabai", APIKey: "key-1", Enabled: true, Transformer: "openai2"},
			{Name: "Rightcode", APIUrl: upstream.URL + "/rightcode", APIKey: "key-2", Enabled: true, Transformer: "openai2"},
		},
	}
	p := New(cfg, nilStatsStorage{}, nil, "test-device")
	if err := p.SetCurrentEndpoint("Rightcode"); err != nil {
		t.Fatalf("set current endpoint: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.handleProxyRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if got := p.GetCurrentEndpointName(); got != "Rightcode" {
		t.Fatalf("expected invalid client request to keep current endpoint Rightcode, got %q", got)
	}
	if got := atomic.LoadInt32(&upstreamHits); got != 0 {
		t.Fatalf("expected invalid client request not to call upstream, got %d calls", got)
	}
}

func TestDefaultProxyTransportUsesEnvironmentProxy(t *testing.T) {
	if os.Getenv("CODE_AGENT_LENS_PROXY_ENV_HELPER") == "1" {
		assertDefaultProxyTransportUsesEnvironment(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultProxyTransportUsesEnvironmentProxy$")
	cmd.Env = append(os.Environ(),
		"CODE_AGENT_LENS_PROXY_ENV_HELPER=1",
		"HTTP_PROXY=",
		"http_proxy=",
		"HTTPS_PROXY=http://127.0.0.1:10808",
		"https_proxy=http://127.0.0.1:10808",
		"ALL_PROXY=",
		"all_proxy=",
		"NO_PROXY=",
		"no_proxy=",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper test failed: %v\n%s", err, string(output))
	}
}

func assertDefaultProxyTransportUsesEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:10808")
	t.Setenv("https_proxy", "http://127.0.0.1:10808")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	transport := NewDefaultProxyTransport()
	if transport.Proxy == nil {
		t.Fatalf("expected default proxy transport to use environment proxy function")
	}
	req := httptest.NewRequest(http.MethodGet, "https://example.test/v1/messages", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}
	if proxyURL == nil {
		t.Fatalf("expected default proxy transport to use HTTPS_PROXY")
	}
	if got, want := proxyURL.String(), "http://127.0.0.1:10808"; got != want {
		t.Fatalf("proxy URL = %q, want %q", got, want)
	}
}

func TestCreateProxyTransportNormalizesProxyURLWithoutScheme(t *testing.T) {
	transport, err := CreateProxyTransport("localhost:10808")
	if err != nil {
		t.Fatalf("CreateProxyTransport returned error: %v", err)
	}
	reqURL, err := url.Parse("https://chatgpt.com/backend-api/codex/v1/responses")
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: reqURL})
	if err != nil {
		t.Fatalf("transport proxy returned error: %v", err)
	}
	if proxyURL == nil {
		t.Fatalf("expected normalized proxy URL")
	}
	if got, want := proxyURL.String(), "http://localhost:10808"; got != want {
		t.Fatalf("proxy URL = %q, want %q", got, want)
	}
}

func TestResolveProxyURLForRequestIgnoresGlobalProxyForRegularEndpoints(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UpdateProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	cfg.UpdateCodexProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})

	reqURL, err := url.Parse("https://api.86gamestore.com/v1/responses")
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}

	if got := resolveProxyURLForRequest(cfg, reqURL); got != "" {
		t.Fatalf("regular endpoint app proxy URL = %q, want none", got)
	}
}

func TestResolveProxyURLForRequestUsesCodexProxyForCodexBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UpdateProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	cfg.UpdateCodexProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})

	reqURL, err := url.Parse("https://chatgpt.com/backend-api/codex/responses")
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}

	if got, want := resolveProxyURLForRequest(cfg, reqURL), "http://127.0.0.1:10808"; got != want {
		t.Fatalf("codex backend proxy URL = %q, want %q", got, want)
	}
}

func TestProxyPersistsCurrentEndpointAcrossRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{
		{Name: "大白AI", APIUrl: "http://example.invalid/dabai", APIKey: "key-1", Enabled: true, Transformer: "openai2"},
		{Name: "Rightcode", APIUrl: "http://example.invalid/rightcode", APIKey: "key-2", Enabled: true, Transformer: "openai2"},
	})
	sqliteStorage, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "code-agent-lens.db"))
	if err != nil {
		t.Fatalf("open sqlite storage: %v", err)
	}
	defer sqliteStorage.Close()

	p := New(cfg, nilStatsStorage{}, sqliteStorage, "test-device")
	if err := p.SetCurrentEndpoint("Rightcode"); err != nil {
		t.Fatalf("set current endpoint: %v", err)
	}

	restarted := New(cfg, nilStatsStorage{}, sqliteStorage, "test-device")

	if got := restarted.GetCurrentEndpointName(); got != "Rightcode" {
		t.Fatalf("expected restored current endpoint Rightcode, got %q", got)
	}
}

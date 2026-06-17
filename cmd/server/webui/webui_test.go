package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
)

func TestFaviconDoesNotFallThroughToProxy(t *testing.T) {
	cfg := config.DefaultConfig()
	p := proxy.New(cfg, nil, nil, "test-device")
	ui := New(cfg, p, nil)
	mux := http.NewServeMux()
	if err := ui.RegisterRoutes(mux); err != nil {
		t.Fatalf("register routes: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected favicon status %d, got %d body=%s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}

func TestRegisterRoutesKeepsConfiguredBasicAuth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BasicAuthEnabled = true
	cfg.BasicAuthUsername = "admin"
	cfg.BasicAuthPassword = "secret"
	p := proxy.New(cfg, nil, nil, "test-device")
	ui := New(cfg, p, nil)
	mux := http.NewServeMux()
	if err := ui.RegisterRoutes(mux); err != nil {
		t.Fatalf("register routes: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected /ui/ status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("expected /ui/ to request basic auth")
	}
}

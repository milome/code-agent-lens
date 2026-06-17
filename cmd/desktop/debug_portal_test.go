package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
)

func TestDesktopDebugPortalEnabledByDefault(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_ENABLED", "")

	if !desktopDebugPortalEnabled() {
		t.Fatalf("expected desktop debug portal to be enabled by default")
	}
}

func TestDesktopDebugPortalCanBeDisabledByEnvironment(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_ENABLED", "false")

	if desktopDebugPortalEnabled() {
		t.Fatalf("expected CODE_AGENT_LENS_OBS_VIEWER_ENABLED=false to disable desktop debug portal")
	}
}

func TestDesktopDebugPortalPortDefaultsTo3011(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_PORT", "")

	if got, want := desktopDebugPortalPort(), 3011; got != want {
		t.Fatalf("desktopDebugPortalPort() = %d, want %d", got, want)
	}
}

func TestDesktopDebugPortalPortUsesEnvironment(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_PORT", "3021")

	if got, want := desktopDebugPortalPort(), 3021; got != want {
		t.Fatalf("desktopDebugPortalPort() = %d, want %d", got, want)
	}
}

func TestBlockDesktopDebugViewerOnGatewayReturnsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	blockDesktopDebugViewerOnGateway(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/obs", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("/debug/obs status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDesktopGatewayRegistersAdminUIWithoutBasicAuth(t *testing.T) {
	mux := http.NewServeMux()
	cfg := config.DefaultConfig()
	cfg.BasicAuthEnabled = true
	cfg.BasicAuthUsername = "admin"
	cfg.BasicAuthPassword = "secret"
	p := proxy.New(cfg, nil, nil, "test-device")

	blockDesktopDebugViewerOnGateway(mux)
	if err := registerDesktopGatewayUI(mux, cfg, p, nil); err != nil {
		t.Fatalf("registerDesktopGatewayUI returned error: %v", err)
	}

	adminRec := httptest.NewRecorder()
	mux.ServeHTTP(adminRec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if adminRec.Code != http.StatusFound {
		t.Fatalf("/admin status = %d, want %d", adminRec.Code, http.StatusFound)
	}
	if got, want := adminRec.Header().Get("Location"), "/ui/"; got != want {
		t.Fatalf("/admin Location = %q, want %q", got, want)
	}

	uiRec := httptest.NewRecorder()
	mux.ServeHTTP(uiRec, httptest.NewRequest(http.MethodGet, "/ui/", nil))

	if uiRec.Code == http.StatusUnauthorized {
		t.Fatalf("/ui/ unexpectedly requested basic auth")
	}
	if got := uiRec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("/ui/ unexpectedly requested basic auth: %q", got)
	}

	apiRec := httptest.NewRecorder()
	mux.ServeHTTP(apiRec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if apiRec.Code == http.StatusUnauthorized {
		t.Fatalf("/api/config unexpectedly requested basic auth")
	}
	if got := apiRec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("/api/config unexpectedly requested basic auth: %q", got)
	}
}

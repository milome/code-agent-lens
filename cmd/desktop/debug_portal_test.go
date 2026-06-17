package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestDesktopGatewayDoesNotRegisterAdminUI(t *testing.T) {
	mux := http.NewServeMux()
	blockDesktopDebugViewerOnGateway(mux)

	for _, path := range []string{"/admin", "/ui/", "/api/config"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != "" {
			t.Fatalf("%s unexpectedly requested basic auth: %q", path, got)
		}
	}
}

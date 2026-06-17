package service

import (
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
)

func TestEndpointServiceResolveProxyURLForTargetIgnoresGlobalProxyForRegularEndpoints(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UpdateProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	cfg.UpdateCodexProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	service := NewEndpointService(cfg, nil, nil)

	if got := service.resolveProxyURLForTarget("https://api.86gamestore.com/v1/responses"); got != "" {
		t.Fatalf("regular endpoint app proxy URL = %q, want none", got)
	}
}

func TestEndpointServiceResolveProxyURLForTargetUsesCodexProxyForCodexBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UpdateProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	cfg.UpdateCodexProxy(&config.ProxyConfig{URL: "http://127.0.0.1:10808"})
	service := NewEndpointService(cfg, nil, nil)

	if got, want := service.resolveProxyURLForTarget("https://chatgpt.com/backend-api/codex/responses"), "http://127.0.0.1:10808"; got != want {
		t.Fatalf("codex backend proxy URL = %q, want %q", got, want)
	}
}

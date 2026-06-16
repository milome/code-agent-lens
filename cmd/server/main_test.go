package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
	"github.com/milome/code-agent-lens/internal/transformer"
	"github.com/milome/code-agent-lens/internal/transformer/convert"
)

func TestHandleObservabilitySmokeUpstreamReturnsJSONForNonStream(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/__obs_smoke/v1/responses", strings.NewReader(`{"stream":false}`))
	rec := httptest.NewRecorder()

	handleObservabilitySmokeUpstream(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"object":"response"`) || !strings.Contains(rec.Body.String(), `"total_tokens":18`) {
		t.Fatalf("response body missing response object: %s", rec.Body.String())
	}
}

func TestHandleObservabilitySmokeUpstreamReturnsResponsesSSEForStream(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/__obs_smoke/v1/responses", strings.NewReader(`{"stream":true}`))
	rec := httptest.NewRecorder()

	handleObservabilitySmokeUpstream(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.content_part.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.content_part.done",
		"event: response.output_item.done",
		"event: response.completed",
		`"type":"response.completed"`,
		`"object":"response"`,
		`"total_tokens":18`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body missing %q: %s", want, body)
		}
	}
}

func TestHandleObservabilitySmokeUpstreamStreamingResponseTransformsToCompleteClaudeSSE(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/__obs_smoke/v1/responses", strings.NewReader(`{"stream":true}`))
	rec := httptest.NewRecorder()

	handleObservabilitySmokeUpstream(rec, req)

	ctx := &transformer.StreamContext{ModelName: "claude-3-5-haiku-20241022"}
	var transformed strings.Builder
	for _, chunk := range strings.Split(rec.Body.String(), "\n\n") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		out, err := convert.OpenAI2StreamToClaude([]byte(chunk+"\n\n"), ctx)
		if err != nil {
			t.Fatalf("OpenAI2StreamToClaude failed for %q: %v", chunk, err)
		}
		transformed.Write(out)
	}

	body := transformed.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"code-agent-lens-otel-smoke-response",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("transformed Claude SSE missing %q: %s", want, body)
		}
	}
}

func TestRuntimeSnapshotProviderSanitizesEndpointInventory(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_RUNTIME_MODE", "docker_compose")

	cfg := config.DefaultConfig()
	cfg.UpdatePort(3010)
	cfg.UpdateEndpoints([]config.Endpoint{
		{
			Name:        "Rightcode",
			APIUrl:      "https://right.codes/v1",
			APIKey:      "sk-secret-rightcode",
			AuthMode:    config.AuthModeAPIKey,
			Enabled:     true,
			Transformer: "openai2",
			Model:       "gpt-5.5",
		},
		{
			Name:        "Disabled",
			APIUrl:      "example.invalid",
			APIKey:      "sk-secret-disabled",
			AuthMode:    config.AuthModeAPIKey,
			Enabled:     false,
			Transformer: "claude",
		},
	})
	p := proxy.New(cfg, nil, nil, "test-device")

	snapshot := runtimeSnapshotProvider(cfg, p, t.TempDir(), 3011)()
	if snapshot.Status != "healthy" {
		t.Fatalf("status = %q, want healthy", snapshot.Status)
	}
	if snapshot.Mode != "docker_compose" {
		t.Fatalf("mode = %q, want docker_compose", snapshot.Mode)
	}
	if snapshot.GatewayListener != "http://127.0.0.1:3010" {
		t.Fatalf("gateway listener = %q", snapshot.GatewayListener)
	}
	if snapshot.PortalListener != "http://127.0.0.1:3011/debug/obs" {
		t.Fatalf("portal listener = %q", snapshot.PortalListener)
	}
	if snapshot.TotalEndpoints != 2 || snapshot.EnabledEndpoints != 1 {
		t.Fatalf("endpoint counts = %d/%d, want 1/2 enabled/total", snapshot.EnabledEndpoints, snapshot.TotalEndpoints)
	}
	if snapshot.ActiveEndpoint == nil || snapshot.ActiveEndpoint.Name != "Rightcode" || snapshot.ActiveEndpoint.BaseURLHost != "right.codes" {
		t.Fatalf("active endpoint = %#v", snapshot.ActiveEndpoint)
	}
	if len(snapshot.Endpoints) != 2 || snapshot.Endpoints[1].Health != "disabled" {
		t.Fatalf("endpoints = %#v", snapshot.Endpoints)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, "sk-secret") || strings.Contains(body, "apiKey") {
		t.Fatalf("runtime snapshot must not expose endpoint secrets: %s", body)
	}
}

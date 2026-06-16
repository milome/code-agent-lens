package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
)

func TestSwitchEndpointUpdatesCurrentProxyEndpoint(t *testing.T) {
	cfg := &config.Config{
		Endpoints: []config.Endpoint{
			{Name: "大白AI", APIUrl: "https://example.invalid/dabai", APIKey: "key-1", Enabled: true, Transformer: "openai2"},
			{Name: "Rightcode", APIUrl: "https://example.invalid/rightcode", APIKey: "key-2", Enabled: true, Transformer: "openai2"},
		},
	}
	p := proxy.New(cfg, nil, nil, "test-device")
	handler := NewHandler(cfg, p, nil)

	switchReq := httptest.NewRequest(http.MethodPost, "/api/endpoints/switch", strings.NewReader(`{"name":"Rightcode"}`))
	switchRec := httptest.NewRecorder()
	handler.ServeHTTP(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("switch status=%d body=%s", switchRec.Code, switchRec.Body.String())
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/api/endpoints/current", nil)
	currentRec := httptest.NewRecorder()
	handler.ServeHTTP(currentRec, currentReq)
	if currentRec.Code != http.StatusOK {
		t.Fatalf("current status=%d body=%s", currentRec.Code, currentRec.Body.String())
	}

	var response struct {
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(currentRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode current response: %v", err)
	}
	if response.Data.Name != "Rightcode" {
		t.Fatalf("expected current endpoint Rightcode after switch, got %q", response.Data.Name)
	}
}

func TestSwitchEndpointRejectsBlankName(t *testing.T) {
	cfg := &config.Config{
		Endpoints: []config.Endpoint{
			{Name: "大白AI", APIUrl: "https://example.invalid/dabai", APIKey: "key-1", Enabled: true, Transformer: "openai2"},
		},
	}
	handler := NewHandler(cfg, proxy.New(cfg, nil, nil, "test-device"), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/endpoints/switch", strings.NewReader(`{"name":"  "}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

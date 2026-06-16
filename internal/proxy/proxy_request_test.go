package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
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

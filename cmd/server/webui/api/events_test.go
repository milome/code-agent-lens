package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
)

func TestEventsReportProxyCurrentEndpoint(t *testing.T) {
	cfg := &config.Config{
		Endpoints: []config.Endpoint{
			{Name: "大白AI", APIUrl: "https://example.invalid/dabai", APIKey: "key-1", Enabled: true, Transformer: "openai2"},
			{Name: "Rightcode", APIUrl: "https://example.invalid/rightcode", APIKey: "key-2", Enabled: true, Transformer: "openai2"},
		},
	}
	p := proxy.New(cfg, nil, nil, "test-device")
	if err := p.SetCurrentEndpoint("Rightcode"); err != nil {
		t.Fatalf("set current endpoint: %v", err)
	}
	handler := NewHandler(cfg, p, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.After(6 * time.Second)
	for {
		if strings.Contains(rec.Body.String(), `"type":"stats"`) {
			cancel()
			<-done
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("timed out waiting for stats event, body=%s", rec.Body.String())
		default:
			time.Sleep(25 * time.Millisecond)
		}
	}

	var statsEvent map[string]any
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") || !strings.Contains(line, `"type":"stats"`) {
			continue
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &statsEvent); err != nil {
			t.Fatalf("decode stats event: %v", err)
		}
		break
	}
	if statsEvent == nil {
		t.Fatalf("stats event not found in body=%s", rec.Body.String())
	}
	if got := statsEvent["currentEndpoint"]; got != "Rightcode" {
		t.Fatalf("expected SSE currentEndpoint Rightcode, got %#v", got)
	}
}

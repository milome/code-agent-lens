package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestProxyObservabilityWrapsHTTPClientTransport(t *testing.T) {
	setProxyFakeOTLPEndpoint(t)
	rt, err := observability.Init(context.Background(), observability.Config{Enabled: true, ServiceName: "code-agent-lens-test"}, "test")
	if err != nil {
		t.Fatalf("Init observability: %v", err)
	}
	defer rt.Shutdown(context.Background())

	p := New(config.DefaultConfig(), nilStatsStorage{}, nil, "test-device")
	p.SetObservabilityRuntime(rt)

	if p.observability != rt {
		t.Fatalf("observability runtime not set")
	}
	if p.httpClient.Transport == nil {
		t.Fatalf("http client transport is nil")
	}
}

func TestSendRequestPropagatesTraceparent(t *testing.T) {
	setProxyFakeOTLPEndpoint(t)
	rt, err := observability.Init(context.Background(), observability.Config{Enabled: true, ServiceName: "code-agent-lens-test"}, "test")
	if err != nil {
		t.Fatalf("Init observability: %v", err)
	}
	defer rt.Shutdown(context.Background())

	var gotTraceparent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ctx, span := rt.Tracer().Start(context.Background(), "parent")
	defer span.End()
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	client := &http.Client{Transport: observability.WrapRoundTripper(http.DefaultTransport)}

	resp, err := sendRequest(ctx, req, client, &config.Config{})
	if err != nil {
		t.Fatalf("sendRequest: %v", err)
	}
	resp.Body.Close()

	if gotTraceparent == "" {
		t.Fatalf("traceparent header was not propagated")
	}
	carrier := propagation.HeaderCarrier(http.Header{"Traceparent": []string{gotTraceparent}})
	sc := otel.GetTextMapPropagator().Extract(context.Background(), carrier)
	if !span.SpanContext().TraceID().IsValid() || span.SpanContext().TraceID() != oteltrace.SpanContextFromContext(sc).TraceID() {
		t.Fatalf("traceparent trace id mismatch: parent=%s header=%s", span.SpanContext().TraceID(), gotTraceparent)
	}
}

func TestMetricNamesMatchContract(t *testing.T) {
	got := observability.MetricNames()
	want := []string{
		"code-agent-lens_proxy_requests_total",
		"code-agent-lens_proxy_errors_total",
		"code-agent-lens_proxy_request_duration_ms",
		"code-agent-lens_upstream_request_duration_ms",
		"code-agent-lens_upstream_retries_total",
		"code-agent-lens_endpoint_rotations_total",
		"code-agent-lens_tokens_input_total",
		"code-agent-lens_tokens_output_total",
		"code-agent-lens_stream_events_total",
		"code-agent-lens_stream_duration_ms",
		"code-agent-lens_credential_refresh_total",
		"code-agent-lens_credential_failures_total",
	}
	if len(got) != len(want) {
		t.Fatalf("MetricNames length=%d want=%d: %v", len(got), len(want), got)
	}
	seen := map[string]bool{}
	for _, name := range got {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("MetricNames missing %s: %v", name, got)
		}
	}
}

func setProxyFakeOTLPEndpoint(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
}

type nilStatsStorage struct{}

func (nilStatsStorage) RecordDailyStat(stat interface{}) error { return nil }
func (nilStatsStorage) GetTotalStats() (int, map[string]interface{}, error) {
	return 0, map[string]interface{}{}, nil
}
func (nilStatsStorage) GetDailyStats(endpointName, startDate, endDate string) ([]interface{}, error) {
	return nil, nil
}
func (nilStatsStorage) GetPeriodStatsAggregated(startDate, endDate string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestLoadConfigFromEnvDefaultsDisabled(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OTEL_ENABLED", "")
	defaultDumpDir := t.TempDir()
	cfg := LoadConfigFromEnv(defaultDumpDir)

	if cfg.Enabled {
		t.Fatalf("Enabled = true, want false")
	}
	if cfg.DumpDir != defaultDumpDir {
		t.Fatalf("DumpDir = %q, want default", cfg.DumpDir)
	}
	if cfg.ViewerPublicURL != "http://127.0.0.1:3011/debug/obs" {
		t.Fatalf("ViewerPublicURL = %q", cfg.ViewerPublicURL)
	}
	if cfg.OTelPromptPreviewBytes != 4096 {
		t.Fatalf("OTelPromptPreviewBytes = %d", cfg.OTelPromptPreviewBytes)
	}
	if cfg.MaxBodyBytes != 0 {
		t.Fatalf("MaxBodyBytes = %d, want unlimited 0", cfg.MaxBodyBytes)
	}
}

func TestLoadConfigFromEnvEnabledAndParsing(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OTEL_ENABLED", "true")
	t.Setenv("CODE_AGENT_LENS_OBS_LOCAL_DEBUG", "true")
	t.Setenv("CODE_AGENT_LENS_OBS_DUMP_ENABLED", "true")
	dumpDir := t.TempDir()
	t.Setenv("CODE_AGENT_LENS_OBS_DUMP_DIR", dumpDir)
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_ENABLED", "true")
	t.Setenv("CODE_AGENT_LENS_OBS_VIEWER_PUBLIC_URL", "http://127.0.0.1:3011/debug/obs")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_HEADERS", "all")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_BODIES", "all")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_SECRETS", "true")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_STREAM_EVENTS", "all")
	t.Setenv("CODE_AGENT_LENS_OBS_MAX_BODY_BYTES", "12345")
	t.Setenv("CODE_AGENT_LENS_OBS_PROMPT_EXTRACT", "true")
	t.Setenv("CODE_AGENT_LENS_OBS_OTEL_PROMPT_MODE", "preview")
	t.Setenv("CODE_AGENT_LENS_OBS_OTEL_PROMPT_PREVIEW_BYTES", "99")

	cfg := LoadConfigFromEnv("ignored")

	if !cfg.Enabled || !cfg.LocalDebug || !cfg.DumpEnabled || !cfg.ViewerEnabled {
		t.Fatalf("boolean envs not parsed: %+v", cfg)
	}
	if cfg.DumpDir != dumpDir || cfg.ViewerPublicURL != "http://127.0.0.1:3011/debug/obs" {
		t.Fatalf("paths not parsed: %+v", cfg)
	}
	if cfg.CaptureHeaders != "all" || cfg.CaptureBodies != "all" || cfg.CaptureStreamEvents != "all" {
		t.Fatalf("capture modes not parsed: %+v", cfg)
	}
	if !cfg.CaptureSecrets || !cfg.PromptExtract {
		t.Fatalf("capture booleans not parsed: %+v", cfg)
	}
	if cfg.MaxBodyBytes != 12345 || cfg.OTelPromptPreviewBytes != 99 {
		t.Fatalf("numeric envs not parsed: %+v", cfg)
	}
	if cfg.OTelPromptMode != "preview" {
		t.Fatalf("OTelPromptMode = %q", cfg.OTelPromptMode)
	}
}

func TestInitReturnsNoopRuntimeWhenDisabled(t *testing.T) {
	rt, err := Init(context.Background(), Config{Enabled: false}, "test")
	if err != nil {
		t.Fatalf("Init disabled error = %v", err)
	}
	if rt == nil {
		t.Fatalf("runtime is nil")
	}
	if rt.RunID() == "" {
		t.Fatalf("RunID is empty")
	}
	if rt.Tracer() == nil || rt.Meter() == nil {
		t.Fatalf("noop tracer/meter not initialized")
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown = %v", err)
	}
}

func TestInitEnabledRuntime(t *testing.T) {
	setFakeOTLPEndpoint(t)
	cfg := Config{
		Enabled:         true,
		ServiceName:     "code-agent-lens-test",
		DumpEnabled:     true,
		DumpDir:         t.TempDir(),
		ViewerEnabled:   true,
		ViewerPublicURL: "http://127.0.0.1:3011/debug/obs",
	}
	rt, err := Init(context.Background(), cfg, "test")
	if err != nil {
		t.Fatalf("Init enabled error = %v", err)
	}
	if rt == nil || rt.TracerProvider() == nil || rt.MeterProvider() == nil {
		t.Fatalf("runtime providers not initialized: %#v", rt)
	}
	if rt.RunID() == "" {
		t.Fatalf("RunID is empty")
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown = %v", err)
	}
}

func TestInitMetricsPrimesContractSeries(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	rt := &Runtime{
		meterProvider: mp,
		meter:         mp.Meter("code-agent-lens"),
	}

	rt.initMetrics()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect metrics: %v", err)
	}

	got := map[string]bool{}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			got[metric.Name] = true
		}
	}

	var missing []string
	for _, name := range MetricNames() {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing primed metrics: %v; got=%v", missing, got)
	}
}

func setFakeOTLPEndpoint(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
}

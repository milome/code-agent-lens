package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/milome/code-agent-lens/internal/observability/dump"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type metricSet struct {
	proxyRequestsTotal        metric.Int64Counter
	proxyErrorsTotal          metric.Int64Counter
	proxyRequestDurationMS    metric.Float64Histogram
	upstreamRequestDurationMS metric.Float64Histogram
	upstreamRetriesTotal      metric.Int64Counter
	endpointRotationsTotal    metric.Int64Counter
	tokensInputTotal          metric.Int64Counter
	tokensOutputTotal         metric.Int64Counter
	streamEventsTotal         metric.Int64Counter
	streamDurationMS          metric.Float64Histogram
	credentialRefreshTotal    metric.Int64Counter
	credentialFailuresTotal   metric.Int64Counter
}

type RequestRecorder struct {
	runtime   *Runtime
	ctx       context.Context
	span      trace.Span
	start     time.Time
	requestID string
	traceID   string
	spanID    string
	obsRef    string
	viewerURL string
	dump      *dump.RequestWriter
	index     dump.PromptIndex
}

type SpanHandle struct {
	ctx  context.Context
	span trace.Span
}

func MetricNames() []string {
	return []string{
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
}

func (r *Runtime) StartRequest(ctx context.Context, req *http.Request, clientFormat string, body []byte) (context.Context, *RequestRecorder) {
	if r == nil {
		return ctx, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := r.Tracer().Start(ctx, "code-agent-lens.request")
	rec := &RequestRecorder{
		runtime:   r,
		ctx:       ctx,
		span:      span,
		start:     time.Now(),
		requestID: uuid.NewString(),
	}
	sc := span.SpanContext()
	rec.traceID = sc.TraceID().String()
	rec.spanID = sc.SpanID().String()
	if rec.traceID == "00000000000000000000000000000000" {
		rec.traceID = uuid.NewString()
	}
	rec.obsRef = rec.traceID + "/" + rec.requestID
	rec.viewerURL = strings.TrimRight(r.cfg.ViewerPublicURL, "/") + "/trace/" + rec.traceID
	span.SetAttributes(
		attribute.String("code-agent-lens_obs_ref", rec.obsRef),
		attribute.String("code-agent-lens_obs_viewer_url", rec.viewerURL),
		attribute.String("code-agent-lens.obs.run_id", r.runID),
		attribute.String("code-agent-lens.obs.request_id", rec.requestID),
		attribute.String("code-agent-lens.obs.trace_id", rec.traceID),
		attribute.String("code-agent-lens.obs.span_id", rec.spanID),
	)
	rec.captureIngress(req, clientFormat, body)
	return ctx, rec
}

func (r *Runtime) initMetrics() {
	if r == nil || r.meter == nil || r.metrics != nil {
		return
	}
	ms := &metricSet{}
	ms.proxyRequestsTotal, _ = r.meter.Int64Counter("code-agent-lens_proxy_requests_total")
	ms.proxyErrorsTotal, _ = r.meter.Int64Counter("code-agent-lens_proxy_errors_total")
	ms.proxyRequestDurationMS, _ = r.meter.Float64Histogram("code-agent-lens_proxy_request_duration_ms")
	ms.upstreamRequestDurationMS, _ = r.meter.Float64Histogram("code-agent-lens_upstream_request_duration_ms")
	ms.upstreamRetriesTotal, _ = r.meter.Int64Counter("code-agent-lens_upstream_retries_total")
	ms.endpointRotationsTotal, _ = r.meter.Int64Counter("code-agent-lens_endpoint_rotations_total")
	ms.tokensInputTotal, _ = r.meter.Int64Counter("code-agent-lens_tokens_input_total")
	ms.tokensOutputTotal, _ = r.meter.Int64Counter("code-agent-lens_tokens_output_total")
	ms.streamEventsTotal, _ = r.meter.Int64Counter("code-agent-lens_stream_events_total")
	ms.streamDurationMS, _ = r.meter.Float64Histogram("code-agent-lens_stream_duration_ms")
	ms.credentialRefreshTotal, _ = r.meter.Int64Counter("code-agent-lens_credential_refresh_total")
	ms.credentialFailuresTotal, _ = r.meter.Int64Counter("code-agent-lens_credential_failures_total")
	r.metrics = ms
	r.primeMetricSeries()
}

func (r *Runtime) primeMetricSeries() {
	if r == nil || r.metrics == nil {
		return
	}
	ctx := context.Background()
	primer := metric.WithAttributes(attribute.Bool("code-agent-lens.primer", true))
	r.metrics.proxyRequestsTotal.Add(ctx, 0, primer)
	r.metrics.proxyErrorsTotal.Add(ctx, 0, primer)
	r.metrics.proxyRequestDurationMS.Record(ctx, 0, primer)
	r.metrics.upstreamRequestDurationMS.Record(ctx, 0, primer)
	r.metrics.upstreamRetriesTotal.Add(ctx, 0, primer)
	r.metrics.endpointRotationsTotal.Add(ctx, 0, primer)
	r.metrics.tokensInputTotal.Add(ctx, 0, primer)
	r.metrics.tokensOutputTotal.Add(ctx, 0, primer)
	r.metrics.streamEventsTotal.Add(ctx, 0, primer)
	r.metrics.streamDurationMS.Record(ctx, 0, primer)
	r.metrics.credentialRefreshTotal.Add(ctx, 0, primer)
	r.metrics.credentialFailuresTotal.Add(ctx, 0, primer)
}

func (r *RequestRecorder) captureIngress(req *http.Request, clientFormat string, body []byte) {
	if r == nil || r.runtime == nil {
		return
	}
	r.WithSpan("code-agent-lens.ingress.capture", func() {
		if r.runtime.cfg.DumpEnabled {
			writer := dump.NewWriter(dump.Config{
				Enabled:              true,
				Root:                 r.runtime.cfg.DumpDir,
				RunID:                r.runtime.runID,
				CaptureHeaders:       r.runtime.cfg.CaptureHeaders,
				CaptureBodies:        r.runtime.cfg.CaptureBodies,
				CaptureSecrets:       r.runtime.cfg.CaptureSecrets,
				CaptureStreamEvents:  r.runtime.cfg.CaptureStreamEvents,
				MaxBodyBytes:         r.runtime.cfg.MaxBodyBytes,
				PromptExtractEnabled: r.runtime.cfg.PromptExtract,
			})
			r.dump, _ = writer.BeginRequest(r.traceID, r.spanID, r.requestID)
			if r.dump != nil && r.dump.Enabled() {
				if strings.EqualFold(r.runtime.cfg.CaptureHeaders, "all") && req != nil {
					_, _ = r.dump.WriteJSONFile("ingress.request.headers.json", dump.RedactHeaders(req.Header, r.runtime.cfg.CaptureSecrets))
				}
				if strings.EqualFold(r.runtime.cfg.CaptureBodies, "all") {
					_, _ = r.dump.WriteFile("ingress.request.body.raw", body)
				}
				if r.runtime.cfg.PromptExtract {
					r.index = dump.ExtractPrompts(clientFormat, body)
					r.writePromptFiles()
					_ = r.dump.WritePromptIndex(r.index)
				}
			}
		}
	})
}

func (r *RequestRecorder) writePromptFiles() {
	if r == nil || r.dump == nil || !r.dump.Enabled() {
		return
	}
	roleSeen := map[string]int{}
	for i := range r.index.Prompts {
		role := safeRole(r.index.Prompts[i].Role)
		roleSeen[role]++
		name := "prompt." + role + ".txt"
		if roleSeen[role] > 1 {
			name = fmt.Sprintf("prompt.%s.%d.txt", role, roleSeen[role])
		}
		rec, err := r.dump.WriteFile(name, []byte(r.index.Prompts[i].Text))
		if err == nil {
			r.index.Prompts[i].File = name
			r.index.Files = append(r.index.Files, rec)
		}
	}
}

func (r *RequestRecorder) WithSpan(name string, fn func()) {
	handle := r.StartSpan(name)
	defer handle.End()
	if fn != nil {
		fn()
	}
}

func (r *RequestRecorder) StartSpan(name string, attrs ...attribute.KeyValue) SpanHandle {
	if r == nil || r.runtime == nil {
		return SpanHandle{ctx: context.Background(), span: trace.SpanFromContext(context.Background())}
	}
	ctx, span := r.runtime.Tracer().Start(r.ctx, name)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return SpanHandle{ctx: ctx, span: span}
}

func (h SpanHandle) Context() context.Context {
	if h.ctx == nil {
		return context.Background()
	}
	return h.ctx
}

func (h SpanHandle) SetAttributes(attrs ...attribute.KeyValue) {
	if h.span != nil && len(attrs) > 0 {
		h.span.SetAttributes(attrs...)
	}
}

func (h SpanHandle) End() {
	if h.span != nil {
		h.span.End()
	}
}

func (h SpanHandle) SetStatus(status string) {
	if h.span == nil || status == "" {
		return
	}
	h.span.SetAttributes(attribute.String("code-agent-lens.status", status))
	if status == "ok" {
		h.span.SetStatus(codes.Ok, status)
	} else if status != "" {
		h.span.SetStatus(codes.Error, status)
	}
}

func (r *RequestRecorder) Context() context.Context {
	if r == nil || r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

func (r *RequestRecorder) RequestID() string {
	if r == nil {
		return ""
	}
	return r.requestID
}
func (r *RequestRecorder) TraceID() string {
	if r == nil {
		return ""
	}
	return r.traceID
}
func (r *RequestRecorder) SpanID() string {
	if r == nil {
		return ""
	}
	return r.spanID
}
func (r *RequestRecorder) ObsRef() string {
	if r == nil {
		return ""
	}
	return r.obsRef
}
func (r *RequestRecorder) ViewerURL() string {
	if r == nil {
		return ""
	}
	return r.viewerURL
}

func (r *RequestRecorder) SetAttributes(attrs ...attribute.KeyValue) {
	if r != nil && r.span != nil && len(attrs) > 0 {
		r.span.SetAttributes(attrs...)
	}
}

func (r *RequestRecorder) SetStatus(status string) {
	if r == nil || r.span == nil {
		return
	}
	r.span.SetAttributes(attribute.String("code-agent-lens.status", status))
	if status == "ok" {
		r.span.SetStatus(codes.Ok, status)
	} else {
		r.span.SetStatus(codes.Error, status)
	}
}

func (r *RequestRecorder) End() {
	if r == nil || r.span == nil {
		return
	}
	r.RecordDuration("code-agent-lens_proxy_request_duration_ms", time.Since(r.start))
	r.span.End()
}

func (r *RequestRecorder) WriteBytes(name string, data []byte) {
	if r == nil || r.dump == nil || !r.dump.Enabled() {
		return
	}
	_, _ = r.dump.WriteFile(name, data)
	_ = r.dump.WritePromptIndex(r.index)
}

func (r *RequestRecorder) WriteJSON(name string, value any) {
	if r == nil || r.dump == nil || !r.dump.Enabled() {
		return
	}
	_, _ = r.dump.WriteJSONFile(name, value)
	_ = r.dump.WritePromptIndex(r.index)
}

func (r *RequestRecorder) WriteHeaders(name string, headers http.Header) {
	if r == nil || r.runtime == nil || !strings.EqualFold(r.runtime.cfg.CaptureHeaders, "all") {
		return
	}
	r.WriteJSON(name, dump.RedactHeaders(headers, r.runtime.cfg.CaptureSecrets))
}

func (r *RequestRecorder) AppendStreamEvent(name string, eventIndex int, data []byte) {
	if r == nil || r.runtime == nil || !strings.EqualFold(r.runtime.cfg.CaptureStreamEvents, "all") {
		return
	}
	_, _ = r.dump.AppendJSONL(name, map[string]any{
		"event_index": eventIndex,
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"bytes":       len(data),
		"sha256":      hashBytes(data),
		"data":        string(data),
	})
	_ = r.dump.WritePromptIndex(r.index)
	r.AddCounter("code-agent-lens_stream_events_total", 1)
}

func (r *RequestRecorder) WriteUsage(inputTokens, outputTokens int, source string) {
	r.WriteJSON("usage.json", map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"source":        source,
	})
	r.AddCounter("code-agent-lens_tokens_input_total", int64(inputTokens))
	r.AddCounter("code-agent-lens_tokens_output_total", int64(outputTokens))
}

func (r *RequestRecorder) AddCounter(name string, value int64) {
	if r == nil || r.runtime == nil || r.runtime.metrics == nil {
		return
	}
	ctx := r.Context()
	switch name {
	case "code-agent-lens_proxy_requests_total":
		r.runtime.metrics.proxyRequestsTotal.Add(ctx, value)
	case "code-agent-lens_proxy_errors_total":
		r.runtime.metrics.proxyErrorsTotal.Add(ctx, value)
	case "code-agent-lens_upstream_retries_total":
		r.runtime.metrics.upstreamRetriesTotal.Add(ctx, value)
	case "code-agent-lens_endpoint_rotations_total":
		r.runtime.metrics.endpointRotationsTotal.Add(ctx, value)
	case "code-agent-lens_tokens_input_total":
		r.runtime.metrics.tokensInputTotal.Add(ctx, value)
	case "code-agent-lens_tokens_output_total":
		r.runtime.metrics.tokensOutputTotal.Add(ctx, value)
	case "code-agent-lens_stream_events_total":
		r.runtime.metrics.streamEventsTotal.Add(ctx, value)
	case "code-agent-lens_credential_refresh_total":
		r.runtime.metrics.credentialRefreshTotal.Add(ctx, value)
	case "code-agent-lens_credential_failures_total":
		r.runtime.metrics.credentialFailuresTotal.Add(ctx, value)
	}
}

func (r *RequestRecorder) RecordDuration(name string, d time.Duration) {
	if r == nil || r.runtime == nil || r.runtime.metrics == nil {
		return
	}
	value := float64(d.Milliseconds())
	ctx := r.Context()
	switch name {
	case "code-agent-lens_proxy_request_duration_ms":
		r.runtime.metrics.proxyRequestDurationMS.Record(ctx, value)
	case "code-agent-lens_upstream_request_duration_ms":
		r.runtime.metrics.upstreamRequestDurationMS.Record(ctx, value)
	case "code-agent-lens_stream_duration_ms":
		r.runtime.metrics.streamDurationMS.Record(ctx, value)
	}
}

func AddCredentialRefreshMetric(success bool) {
	rt := Current()
	if rt == nil || rt.metrics == nil {
		return
	}
	if success {
		rt.metrics.credentialRefreshTotal.Add(context.Background(), 1)
		return
	}
	rt.metrics.credentialFailuresTotal.Add(context.Background(), 1)
}

func safeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "unknown"
	}
	role = strings.ReplaceAll(role, "/", "_")
	role = strings.ReplaceAll(role, "\\", "_")
	return role
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

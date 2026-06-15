package observability

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// Runtime owns OTel providers and local debug observability state.
type Runtime struct {
	cfg            Config
	runID          string
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	tracer         trace.Tracer
	meter          metric.Meter
	metrics        *metricSet
	shutdown       func(context.Context) error
}

var (
	currentMu      sync.RWMutex
	currentRuntime *Runtime
)

// Init initializes observability providers. Disabled config returns a no-op runtime.
func Init(ctx context.Context, cfg Config, version string) (*Runtime, error) {
	runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + uuid.NewString()
	if cfg.ServiceName == "" {
		cfg.ServiceName = "code-agent-lens"
	}

	if !cfg.Enabled {
		rt := &Runtime{
			cfg:            cfg,
			runID:          runID,
			tracerProvider: trace.NewNoopTracerProvider(),
			meterProvider:  noop.NewMeterProvider(),
			shutdown:       func(context.Context) error { return nil },
		}
		rt.tracer = rt.tracerProvider.Tracer("code-agent-lens")
		rt.meter = rt.meterProvider.Meter("code-agent-lens")
		rt.initMetrics()
		SetCurrent(rt)
		return rt, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			attribute.String("service.version", version),
			attribute.String("code-agent-lens.obs.run_id", runID),
		),
	)
	if err != nil {
		return nil, err
	}

	otlpEndpoint := resolveOTLPEndpoint()
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(otlpEndpoint))
	if err != nil {
		return nil, err
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(otlpEndpoint))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(traceExporter),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	rt := &Runtime{
		cfg:            cfg,
		runID:          runID,
		tracerProvider: tp,
		meterProvider:  mp,
		shutdown: func(shutdownCtx context.Context) error {
			return errors.Join(tp.Shutdown(shutdownCtx), mp.Shutdown(shutdownCtx))
		},
	}
	rt.tracer = rt.tracerProvider.Tracer("code-agent-lens")
	rt.meter = rt.meterProvider.Meter("code-agent-lens")
	rt.initMetrics()
	SetCurrent(rt)
	_ = ctx
	return rt, nil
}

func resolveOTLPEndpoint() string {
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return "http://127.0.0.1:4318"
}

func SetCurrent(rt *Runtime) {
	currentMu.Lock()
	defer currentMu.Unlock()
	currentRuntime = rt
}

func Current() *Runtime {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return currentRuntime
}

func (r *Runtime) Config() Config {
	if r == nil {
		return Config{}
	}
	return r.cfg
}

func (r *Runtime) RunID() string {
	if r == nil {
		return ""
	}
	return r.runID
}

func (r *Runtime) TracerProvider() trace.TracerProvider {
	if r == nil {
		return trace.NewNoopTracerProvider()
	}
	return r.tracerProvider
}

func (r *Runtime) MeterProvider() metric.MeterProvider {
	if r == nil {
		return noop.NewMeterProvider()
	}
	return r.meterProvider
}

func (r *Runtime) Tracer() trace.Tracer {
	if r == nil {
		return trace.NewNoopTracerProvider().Tracer("code-agent-lens")
	}
	return r.tracer
}

func (r *Runtime) Meter() metric.Meter {
	if r == nil {
		return noop.NewMeterProvider().Meter("code-agent-lens")
	}
	return r.meter
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil || r.shutdown == nil {
		return nil
	}
	return r.shutdown(ctx)
}

func (r *Runtime) WrapHandler(handler http.Handler, name string) http.Handler {
	if handler == nil {
		handler = http.DefaultServeMux
	}
	if r == nil || !r.cfg.Enabled {
		return handler
	}
	if name == "" {
		name = "code-agent-lens.http"
	}
	return otelhttp.NewHandler(handler, name)
}

func WrapRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := Current()
	if rt == nil || !rt.cfg.Enabled {
		return base
	}
	return otelhttp.NewTransport(base)
}

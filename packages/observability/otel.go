// Package observability is the platform-wide OpenTelemetry bootstrap for
// Go services. It wires tracer + meter providers to an OTLP/HTTP collector
// (per docs/operations/monitoring-alerts.md — Tempo + VictoriaMetrics via
// the platform's otel-collector) and degrades gracefully to no-op providers
// when the collector endpoint is absent or unreachable.
//
// Typical use from a service main:
//
//	prov, err := observability.Init(ctx, observability.Config{
//	    ServiceName: "tenant-service",
//	    Version:     buildVersion,
//	    Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
//	    Logger:      log,
//	})
//	if err != nil { /* fail-fast */ }
//	defer prov.Shutdown(context.Background())
//
//	r := chi.NewRouter()
//	r.Use(observability.ChiMiddleware("tenant-service"))
//
// If Endpoint is empty the provider is constructed in "noop" mode: the
// global tracer/meter providers stay as the SDK no-ops, no exporter is
// started, and Shutdown is a cheap no-op. Services therefore run in dev
// without an otel-collector.
package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config controls Init. ServiceName is required when the endpoint is set.
type Config struct {
	// ServiceName populates resource attribute `service.name` (Loki label
	// `service` and Tempo span attribute). Required when Endpoint is non-empty.
	ServiceName string
	// Version populates `service.version` (build metadata). Optional.
	Version string
	// Environment populates `deployment.environment` (e.g. "dev", "prod").
	// Optional; falls back to "unknown".
	Environment string
	// Endpoint is the OTLP/HTTP collector endpoint, e.g.
	// "http://otel-collector.platform.svc:4318". Empty means no-op mode.
	// Both bare host:port and full URLs are accepted.
	Endpoint string
	// SampleRatio is the parent-based ratio sampler probability for traces.
	// 0 → use 0.1 default (per monitoring-alerts.md "Sampling: 10% production").
	// Set to 1.0 in staging.
	SampleRatio float64
	// Logger is used for warn/error messages. Defaults to slog.Default().
	Logger *slog.Logger
	// ExportTimeout caps a single OTLP export call. Defaults to 10s.
	ExportTimeout time.Duration
}

// Provider holds the SDK objects so the service can shut them down.
// A zero Provider (returned in no-op mode) has a safe Shutdown.
type Provider struct {
	tracerProvider *trace.TracerProvider
	meterProvider  *metric.MeterProvider
	serviceName    string
	noop           bool
	logger         *slog.Logger
}

// Init configures global tracer + meter providers and returns a Provider
// owning their lifecycle. Callers MUST defer prov.Shutdown(ctx) so spans
// and metrics flush on graceful shutdown.
//
// A nil error is returned when the endpoint is empty/invalid — the caller
// gets a no-op provider and a logged warning. This keeps `go run` and unit
// tests usable without an otel-collector. Genuine misconfiguration (e.g.
// missing ServiceName when an endpoint IS set) returns an error.
func Init(ctx context.Context, cfg Config) (*Provider, error) {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		log.Warn("observability: OTEL_EXPORTER_OTLP_ENDPOINT не задан, телеметрия отключена",
			"service", cfg.ServiceName)
		return &Provider{noop: true, serviceName: cfg.ServiceName, logger: log}, nil
	}

	if cfg.ServiceName == "" {
		return nil, errors.New("observability: ServiceName обязателен, когда задан Endpoint")
	}

	host, insecure, err := parseOTLPEndpoint(endpoint)
	if err != nil {
		// Don't fail the service for a malformed endpoint — fall back to no-op
		// and warn loudly. Operators see the warn in Loki.
		log.Warn("observability: некорректный OTLP endpoint, телеметрия отключена",
			"endpoint", endpoint, "err", err)
		return &Provider{noop: true, serviceName: cfg.ServiceName, logger: log}, nil
	}

	exportTimeout := cfg.ExportTimeout
	if exportTimeout <= 0 {
		exportTimeout = 10 * time.Second
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	traceOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithTimeout(exportTimeout),
	}
	if insecure {
		traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
	}
	traceExp, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("observability: create trace exporter: %w", err)
	}

	ratio := cfg.SampleRatio
	if ratio <= 0 {
		ratio = 0.1
	}
	if ratio > 1 {
		ratio = 1
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExp,
			trace.WithBatchTimeout(5*time.Second),
			trace.WithExportTimeout(exportTimeout),
		),
		trace.WithResource(res),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(ratio))),
	)

	metricOpts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(host),
		otlpmetrichttp.WithTimeout(exportTimeout),
	}
	if insecure {
		metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
	}
	metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
	if err != nil {
		// Trace-only mode: shut down the trace provider too so we don't leak.
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("observability: create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExp,
			metric.WithInterval(30*time.Second),
			metric.WithTimeout(exportTimeout),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info("observability: OTLP exporters инициализированы",
		"service", cfg.ServiceName,
		"endpoint", host,
		"sample_ratio", ratio)

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		serviceName:    cfg.ServiceName,
		logger:         log,
	}, nil
}

// Shutdown flushes pending spans/metrics. Safe to call on a nil or no-op
// Provider — both are common in tests.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.noop {
		return nil
	}
	var errs []error
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace shutdown: %w", err))
		}
	}
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metric shutdown: %w", err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ServiceName returns the configured service name (useful for middleware
// callers that don't want to thread the value around separately).
func (p *Provider) ServiceName() string {
	if p == nil {
		return ""
	}
	return p.serviceName
}

// IsNoop reports whether the provider is in no-op mode (no exporters).
func (p *Provider) IsNoop() bool {
	return p == nil || p.noop
}

// parseOTLPEndpoint accepts both "host:4318" and "http(s)://host:4318" and
// returns (host, insecure, err). The OTLP/HTTP exporter takes a bare
// host:port plus a separate WithInsecure() option.
func parseOTLPEndpoint(raw string) (host string, insecure bool, err error) {
	if !strings.Contains(raw, "://") {
		// bare host:port — assume http (collector default in-cluster).
		return raw, true, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, err
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("empty host in %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return u.Host, true, nil
	case "https":
		return u.Host, false, nil
	default:
		return "", false, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

// buildResource attaches service.name/version/environment plus telemetry
// SDK attributes — these become Loki labels and Tempo span attributes.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	env := cfg.Environment
	if env == "" {
		env = "unknown"
	}
	attrs := []any{
		semconv.ServiceName(cfg.ServiceName),
		semconv.DeploymentEnvironment(env),
	}
	if cfg.Version != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.Version))
	}
	// resource.New returns a partially-initialised resource even on a
	// "host detection failed" error — keep the merge instead of bailing.
	base, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(toAttributes(attrs)...),
	)
	if err != nil && base == nil {
		return nil, err
	}
	return base, nil
}

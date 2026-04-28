# observability

Shared OpenTelemetry bootstrap for AIbank Go services. One module, two
files: `Init` builds tracer + meter providers wired to the platform OTLP
collector; `ChiMiddleware` wraps each chi handler in a server span.

The platform observability stack (LGTM) is documented in
[`docs/operations/monitoring-alerts.md`](../../docs/operations/monitoring-alerts.md).
Spans go to Tempo, metrics to VictoriaMetrics via the `otel-collector`
service.

## Why a shared package?

Per `docs/security-architecture.md` § 8 every service emits structured
logs with `service` / `tenant_id` / `level` / `trace_id`. Repeating the
SDK setup in every `main.go` (resource attrs, batcher tuning, sampler,
graceful shutdown) is the usual way drift creeps in. One package means:

- one place to evolve as exporters / SDK semantic conventions change;
- consistent resource attributes (`service.name`, `service.version`,
  `deployment.environment`) → consistent Loki labels and Tempo joins;
- safe defaults (parent-based 10% sampler, 30 s metric push, 5 s span
  batcher, 10 s export timeout);
- graceful no-op fallback so `go run` and unit tests work without an
  otel-collector.

## Usage

`module github.com/aibank/platform/packages/observability`. Plug in via a
`replace` directive in the consumer's `go.mod`:

```go
require github.com/aibank/platform/packages/observability v0.0.0
replace github.com/aibank/platform/packages/observability => ../../packages/observability
```

In `main.go`:

```go
prov, err := observability.Init(ctx, observability.Config{
    ServiceName: "tenant-service",
    Version:     os.Getenv("OTEL_SERVICE_VERSION"),
    Environment: os.Getenv("DEPLOY_ENV"),
    Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
    Logger:      log,
})
if err != nil { /* fail-fast */ }
defer prov.Shutdown(context.Background())

r := chi.NewRouter()
r.Use(observability.ChiMiddleware("tenant-service"))
```

## Environment

| Var | Default | Notes |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (empty → no-op) | `http://otel-collector.platform.svc:4318` in cluster |
| `OTEL_SERVICE_VERSION` | (empty) | Build SHA / semver, surfaced as `service.version` |
| `DEPLOY_ENV` | `unknown` | `dev` / `staging` / `prod` |
| `OTEL_TRACES_SAMPLER` etc. | honored via `resource.WithFromEnv()` | Standard OTel env vars are picked up |

## No-op fallback

If `OTEL_EXPORTER_OTLP_ENDPOINT` is empty **or** malformed, `Init` logs a
warning and returns a no-op provider. The global tracer/meter providers
stay at the SDK defaults (no-op), `Shutdown` is a cheap return, and the
chi middleware still runs but produces no exported data. Services
therefore start cleanly in dev without an otel-collector.

## Tests

```bash
cd packages/observability
go test ./...
```

## Related

- [`docs/operations/monitoring-alerts.md`](../../docs/operations/monitoring-alerts.md) — LGTM stack, SLI/SLO
- [`docs/security-architecture.md`](../../docs/security-architecture.md) § 8 — what gets logged
- `packages/audit-sdk/` — sibling package using the same `replace`-directive pattern

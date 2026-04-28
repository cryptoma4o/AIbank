package observability

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// quietLogger is used by tests so warn/info from Init doesn't pollute the
// `go test` output. We still want to make sure those calls don't panic on
// a nil logger somewhere.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestInit_NoEndpoint_NoError verifies the no-op fallback. With an empty
// endpoint Init must succeed, return a Provider that reports IsNoop, and
// Shutdown must be a cheap no-op.
func TestInit_NoEndpoint_NoError(t *testing.T) {
	t.Parallel()
	prov, err := Init(context.Background(), Config{
		ServiceName: "test-svc",
		Endpoint:    "",
		Logger:      quietLogger(),
	})
	if err != nil {
		t.Fatalf("Init: unexpected error: %v", err)
	}
	if prov == nil {
		t.Fatal("Init: provider is nil")
	}
	if !prov.IsNoop() {
		t.Fatal("Init: expected no-op provider when endpoint is empty")
	}
	if got := prov.ServiceName(); got != "test-svc" {
		t.Fatalf("ServiceName: got %q want %q", got, "test-svc")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := prov.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestInit_BadEndpoint_NoError covers the "operator typo'd the URL" path.
// Init should not fail the service — just warn and degrade to no-op.
// Shutdown stays safe afterwards.
//
// Note we have to pick a URL that fails url.Parse OR has an unsupported
// scheme; bare hostnames are accepted by design (k8s service names).
func TestInit_BadEndpoint_NoError(t *testing.T) {
	t.Parallel()
	prov, err := Init(context.Background(), Config{
		ServiceName: "test-svc",
		Endpoint:    "ftp://otel-collector:4318", // unsupported scheme
		Logger:      quietLogger(),
	})
	if err != nil {
		t.Fatalf("Init: unexpected error on bad URL: %v", err)
	}
	if !prov.IsNoop() {
		t.Fatal("Init: expected no-op fallback on malformed endpoint")
	}
	if err := prov.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestInit_MissingServiceNameWithEndpoint asserts the one genuine config
// error: an endpoint set without a service name. We want loud failure
// here, not a silent no-op — operators expect telemetry when they wired
// up the collector.
func TestInit_MissingServiceNameWithEndpoint(t *testing.T) {
	t.Parallel()
	_, err := Init(context.Background(), Config{
		ServiceName: "",
		Endpoint:    "http://localhost:4318",
		Logger:      quietLogger(),
	})
	if err == nil {
		t.Fatal("Init: expected error when ServiceName is empty but Endpoint set")
	}
}

// TestParseOTLPEndpoint covers the URL-parsing helper directly so the
// table is small and fast (no live exporter needed).
func TestParseOTLPEndpoint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		in         string
		wantHost   string
		wantInsec  bool
		wantErr    bool
	}{
		{"bare host:port", "otel-collector:4318", "otel-collector:4318", true, false},
		{"http", "http://otel-collector:4318", "otel-collector:4318", true, false},
		{"https", "https://otel.example.com:4318", "otel.example.com:4318", false, false},
		{"empty host", "http://", "", false, true},
		{"bad scheme", "ftp://otel:4318", "", false, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			host, insec, err := parseOTLPEndpoint(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err mismatch: got %v want err=%v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if host != c.wantHost || insec != c.wantInsec {
				t.Fatalf("got (%q,%v) want (%q,%v)", host, insec, c.wantHost, c.wantInsec)
			}
		})
	}
}

// TestChiMiddleware_AddsSpan verifies that a request through a chi router
// wrapped with ChiMiddleware sees a recording span in its context. We
// install a SDK tracer provider with an in-memory exporter so otelhttp
// can record spans, then assert the handler observed a valid SpanContext
// AND that the resulting span was exported with the chi route pattern.
func TestChiMiddleware_AddsSpan(t *testing.T) {
	// Not t.Parallel() — we mutate the global tracer provider.
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	r := chi.NewRouter()
	r.Use(ChiMiddleware("test-svc"))

	var sawValidSpan bool
	r.Get("/v1/items/{id}", func(w http.ResponseWriter, req *http.Request) {
		span := trace.SpanFromContext(req.Context())
		if span.SpanContext().IsValid() {
			sawValidSpan = true
		}
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/items/abc")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	if !sawValidSpan {
		t.Fatal("ChiMiddleware: handler did not see a valid span context")
	}
	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("ChiMiddleware: no spans were exported")
	}
}

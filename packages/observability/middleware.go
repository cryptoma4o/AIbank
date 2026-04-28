package observability

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ChiMiddleware returns chi-compatible middleware that wraps each request
// in an OTel server span. It uses otelhttp under the hood (the official
// contrib instrumentation) so context propagation, status mapping, and
// metrics-via-attributes all work the way the rest of the otel ecosystem
// expects.
//
// Span name strategy: after chi has matched the route we relabel the span
// with the route pattern (e.g. "GET /v1/tenants/{id}") instead of the raw
// URL — keeps Tempo / VM cardinality bounded. If the route hasn't been
// matched yet (e.g. 404), we fall back to "<method> <path>".
//
// serviceName is also written as `http.server.name` so Tempo's service
// graph picks the same label that Loki uses (`service` label).
func ChiMiddleware(serviceName string) func(http.Handler) http.Handler {
	if serviceName == "" {
		serviceName = "unknown-service"
	}

	return func(next http.Handler) http.Handler {
		instrumented := otelhttp.NewHandler(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// chi matches the route inside its own middleware/mux —
				// after next.ServeHTTP returns we read the matched
				// pattern. To label the span correctly we set the name
				// before delegating using the pattern that chi can
				// already report (RoutePattern is empty until after
				// match, but RouteContext is populated as middlewares
				// run).
				rc := chi.RouteContext(r.Context())
				span := trace.SpanFromContext(r.Context())
				if rc != nil && rc.RoutePattern() != "" {
					span.SetName(r.Method + " " + rc.RoutePattern())
					span.SetAttributes(semconv.HTTPRoute(rc.RoutePattern()))
				}
				if serviceName != "" {
					span.SetAttributes(attribute.String("http.server.name", serviceName))
				}
				next.ServeHTTP(w, r)

				// chi may only have set the pattern by now (mux runs
				// middlewares before match). Update the span if so.
				if rc != nil && rc.RoutePattern() != "" {
					span.SetName(r.Method + " " + rc.RoutePattern())
					span.SetAttributes(semconv.HTTPRoute(rc.RoutePattern()))
				}
			}),
			serviceName,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				// Initial name before the route is matched. ChiMiddleware
				// rewrites it once chi has resolved the pattern.
				return r.Method + " " + r.URL.Path
			}),
		)
		return instrumented
	}
}

// toAttributes converts a heterogeneous slice (string/int/etc) into
// attribute.KeyValue, used by buildResource. Centralising it here keeps
// the otel.go file readable.
func toAttributes(in []any) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(in))
	for _, v := range in {
		if kv, ok := v.(attribute.KeyValue); ok {
			out = append(out, kv)
		}
	}
	return out
}

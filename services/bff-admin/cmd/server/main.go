// Package main — bff-admin HTTP-сервер (port 8092).
//
// Endpoints:
//
//	POST /graphql    — GraphQL (требует bank.admin / platform.admin)
//	GET  /playground — GraphiQL UI (только при GRAPHQL_PLAYGROUND=1)
//	GET  /health     — liveness
//	GET  /ready      — readiness
//
// ENV:
//
//	JWT_SECRET           — общий с identity-service (>=32 байт)
//	TENANT_SERVICE_URL, AUDIT_SERVICE_URL, ORCHESTRATOR_URL,
//	RISK_ENGINE_URL, IDENTITY_SERVICE_URL — URL'ы downstream
//	GRAPHQL_PLAYGROUND   — "1" чтобы включить /playground
//	PORT                 — по умолчанию 8092
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/graphql-go/handler"

	obs "github.com/aibank/platform/packages/observability"

	"aibank/bff-admin/graph"
	"aibank/bff-admin/internal/auth"
	"aibank/bff-admin/internal/clients"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "bff-admin",
		Version:     os.Getenv("OTEL_SERVICE_VERSION"),
		Environment: os.Getenv("DEPLOY_ENV"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Logger:      log,
	})
	obsCancel()
	if err != nil {
		log.Error("init observability", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = obsProvider.Shutdown(shutCtx)
	}()

	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		log.Error("JWT_SECRET must be set and >=32 bytes")
		os.Exit(1)
	}
	verifier, err := auth.NewVerifier([]byte(secret))
	if err != nil {
		log.Error("init JWT verifier", "err", err)
		os.Exit(1)
	}

	resolver := graph.NewResolver(
		clients.NewTenantClient(env("TENANT_SERVICE_URL", "http://tenant-service:8080")),
		clients.NewAuditClient(env("AUDIT_SERVICE_URL", "http://audit-service:8081")),
		clients.NewOrchestratorClient(env("ORCHESTRATOR_URL", "http://onboarding-orchestrator:8085")),
		clients.NewRiskClient(env("RISK_ENGINE_URL", "http://risk-engine:8086")),
		clients.NewIdentityClient(env("IDENTITY_SERVICE_URL", "http://identity-service:8082")),
	)
	schema, err := resolver.Schema()
	if err != nil {
		log.Error("build GraphQL schema", "err", err)
		os.Exit(1)
	}

	gqlHandler := handler.New(&handler.Config{Schema: &schema, Pretty: false, GraphiQL: false})
	playgroundEnabled := strings.EqualFold(os.Getenv("GRAPHQL_PLAYGROUND"), "1") ||
		strings.EqualFold(os.Getenv("GRAPHQL_PLAYGROUND"), "true")

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(obs.ChiMiddleware("bff-admin"))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	r.Route("/graphql", func(sub chi.Router) {
		sub.Use(auth.Middleware(verifier))
		sub.Handle("/", gqlHandler)
	})

	if playgroundEnabled {
		r.Get("/playground", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(playgroundHTML))
		})
		log.Info("GraphQL playground enabled at /playground")
	}

	port := env("PORT", "8092")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("bff-admin listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// playgroundHTML — минимальный self-contained GraphiQL для dev.
const playgroundHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"/><title>bff-admin playground</title>
<link rel="stylesheet" href="https://unpkg.com/graphiql/graphiql.min.css"/>
</head><body style="margin:0"><div id="graphiql" style="height:100vh"></div>
<script src="https://unpkg.com/react/umd/react.production.min.js"></script>
<script src="https://unpkg.com/react-dom/umd/react-dom.production.min.js"></script>
<script src="https://unpkg.com/graphiql/graphiql.min.js"></script>
<script>
const fetcher = GraphiQL.createFetcher({ url: '/graphql' });
ReactDOM.render(React.createElement(GraphiQL, { fetcher }), document.getElementById('graphiql'));
</script></body></html>`

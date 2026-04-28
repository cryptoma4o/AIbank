// Package main — bff-onboarding HTTP-сервер.
//
// Endpoints:
//   - POST /graphql    — GraphQL (требует Bearer access-token)
//   - GET  /playground — GraphQL Playground (только при GRAPHQL_PLAYGROUND=1)
//   - GET  /health     — liveness
//   - GET  /ready      — readiness
//
// Конфигурация — через ENV (см. README/AGENTS.md):
//   - JWT_SECRET (required, >=32 байт)
//   - TENANT_SERVICE_URL, IDENTITY_SERVICE_URL, DOCUMENT_SERVICE_URL,
//     ORCHESTRATOR_URL, RISK_ENGINE_URL — URL'ы downstream-сервисов
//   - GRAPHQL_PLAYGROUND=1 — включить /playground (по умолчанию выключен)
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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/graphql-go/handler"

	obs "github.com/aibank/platform/packages/observability"

	"aibank/bff-onboarding/graph"
	"aibank/bff-onboarding/internal/auth"
	"aibank/bff-onboarding/internal/clients"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
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
		ServiceName: "bff-onboarding",
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
		log.Error("JWT_SECRET must be set and >=32 bytes (RFC 7518)")
		os.Exit(1)
	}
	verifier, err := auth.NewVerifier([]byte(secret))
	if err != nil {
		log.Error("init JWT verifier", "err", err)
		os.Exit(1)
	}

	tenantURL := env("TENANT_SERVICE_URL", "http://tenant-service:8080")
	identityURL := env("IDENTITY_SERVICE_URL", "http://identity-service:8082")
	documentURL := env("DOCUMENT_SERVICE_URL", "http://document-service:8083")
	orchestratorURL := env("ORCHESTRATOR_URL", "http://onboarding-orchestrator:8085")
	riskURL := env("RISK_ENGINE_URL", "http://risk-engine:8086")

	resolver := graph.NewResolver(
		clients.NewTenantClient(tenantURL),
		clients.NewIdentityClient(identityURL),
		clients.NewDocumentClient(documentURL),
		clients.NewOrchestratorClient(orchestratorURL),
		clients.NewRiskClient(riskURL),
	)
	schema, err := resolver.Schema()
	if err != nil {
		log.Error("build GraphQL schema", "err", err)
		os.Exit(1)
	}

	gqlHandler := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   false,
		GraphiQL: false,
	})
	playgroundEnabled := strings.EqualFold(os.Getenv("GRAPHQL_PLAYGROUND"), "1") ||
		strings.EqualFold(os.Getenv("GRAPHQL_PLAYGROUND"), "true")

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("bff-onboarding"))
	r.Use(middleware.Timeout(30 * time.Second))

	// Liveness/readiness — без auth.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// /graphql — основной endpoint, под JWT-middleware.
	r.Route("/graphql", func(sub chi.Router) {
		sub.Use(auth.Middleware(verifier))
		sub.Handle("/", gqlHandler)
	})

	// /playground — отладочный UI, только в dev.
	if playgroundEnabled {
		r.Get("/playground", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(playgroundHTML))
		})
		log.Info("GraphQL playground enabled at /playground")
	}

	port := env("PORT", "8091")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("bff-onboarding listening", "addr", srv.Addr)
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
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", "err", err)
	}
}

// playgroundHTML — минимальный self-contained GraphiQL.
//
// CDN-загрузка приемлема в dev; в production /playground выключен.
const playgroundHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"/><title>bff-onboarding playground</title>
<link rel="stylesheet" href="https://unpkg.com/graphiql/graphiql.min.css"/>
</head><body style="margin:0"><div id="graphiql" style="height:100vh"></div>
<script src="https://unpkg.com/react/umd/react.production.min.js"></script>
<script src="https://unpkg.com/react-dom/umd/react-dom.production.min.js"></script>
<script src="https://unpkg.com/graphiql/graphiql.min.js"></script>
<script>
const fetcher = GraphiQL.createFetcher({ url: '/graphql' });
ReactDOM.render(React.createElement(GraphiQL, { fetcher }), document.getElementById('graphiql'));
</script></body></html>`

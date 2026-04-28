package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"
	"github.com/aibank/platform/packages/secrets"
	"github.com/aibank/platform/services/tenant-service/internal/audit"
	"github.com/aibank/platform/services/tenant-service/internal/handler"
	"github.com/aibank/platform/services/tenant-service/internal/repository"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "tenant-service",
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

	// Resolve DATABASE_URL via secrets provider (Vault in prod, env in dev).
	// Direct DATABASE_URL env var still wins for backward-compat / dev workflow,
	// per docs/security-architecture.md § 7.3 + ADR-0002.
	//
	// Env contract:
	//   DATABASE_URL              full DSN (dev shortcut, takes precedence)
	//   DATABASE_URL_SECRET_KEY   secrets-provider key, default "database/url"
	//   SECRETS_BACKEND           env | vault | chained (default chained)
	//   VAULT_ADDR / VAULT_TOKEN  Vault wiring (see packages/secrets/README.md)
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		provider, err := secrets.BuildProvider(secrets.FactoryConfig{
			ServiceName: "tenant-service",
		})
		if err != nil {
			log.Error("init secrets provider", "err", err)
			os.Exit(1)
		}
		key := os.Getenv("DATABASE_URL_SECRET_KEY")
		if key == "" {
			key = "database/url"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		dsn, err = provider.GetSecret(ctx, key)
		cancel()
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				log.Error("DATABASE_URL not configured (env or secrets backend)", "key", key)
			} else {
				log.Error("fetch DATABASE_URL secret", "err", err, "key", key)
			}
			os.Exit(1)
		}
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	defer db.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(pingCtx); err != nil {
		pingCancel()
		log.Error("ping db", "err", err)
		os.Exit(1)
	}
	pingCancel()

	tenantRepo := repository.NewPostgresTenantRepository(db)
	configRepo := repository.NewPostgresTenantConfigRepository(db)
	provisioner := repository.NewPostgresSchemaProvisioner(db)

	// Audit-клиент: best-effort, см. ADR-0010.  При отсутствии переменной
	// AUDIT_SERVICE_URL логируем предупреждение и продолжаем — это
	// допустимо в DEV-сценариях без audit-service.
	auditClient := audit.MustClient(log)
	tenantHandler := handler.NewTenantHandler(tenantRepo, configRepo, provisioner, auditClient, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("tenant-service"))
	r.Use(middleware.Timeout(30 * time.Second))

	// Структурированные probe-эндпоинты на packages/healthz.
	// /health (liveness) — всегда 200, если процесс жив; не зависит от внешних систем.
	// /ready (readiness) — 503, если хотя бы одна обязательная проверка fail.
	hc := healthz.New("tenant-service", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1/tenants", tenantHandler.Routes())

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("tenant-service starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Info("stopped")
}

package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"

	"aibank/billing-service/internal/audit"
	"aibank/billing-service/internal/clients"
	"aibank/billing-service/internal/handler"
	"aibank/billing-service/internal/relay"
	"aibank/billing-service/internal/repository"
)

const (
	listenAddr      = ":8087"
	shutdownTimeout = 15 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "billing-service",
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

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	tenantURL := os.Getenv("TENANT_SERVICE_URL")
	if tenantURL == "" {
		log.Warn("TENANT_SERVICE_URL не задан — все тенанты будут получать tier=basic")
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

	// Outbox-relay (ADR-0010 § 2). При пустом KAFKA_BROKERS Bundle.Relay==nil
	// и Run() сразу выходит — это dev-режим без Kafka.
	relayBundle, err := relay.Build(db, relay.LoadConfigFromEnv(), log)
	if err != nil {
		log.Error("build outbox relay", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := relayBundle.Close(); cerr != nil {
			log.Error("close outbox publisher", "err", cerr)
		}
	}()

	repo := repository.NewPostgresBillingEventRepository(db)
	if relayBundle.Outbox != nil {
		// transactional outbox активен: Append будет писать в outbox в одной
		// транзакции с INSERT'ом в platform.billing_events.
		repo = repo.WithOutbox(relayBundle.Outbox)
		log.Info("billing-service: transactional outbox активен")
	}
	tenantClient := clients.NewTenantClient(tenantURL, log)
	auditClient := audit.MustClient(log)
	eventHandler := handler.NewEventHandler(repo, tenantClient, auditClient, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("billing-service"))
	r.Use(middleware.Timeout(30 * time.Second))

	// Структурированные probe-эндпоинты на packages/healthz.
	// tenant-service — soft-проверка: при недоступности billing работает в
	// degraded-режиме (tier=basic для всех тенантов), но readiness не теряем.
	hc := healthz.New("billing-service", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))
	tenantHealthURL := tenantURL
	if tenantHealthURL == "" {
		tenantHealthURL = "http://tenant-service:8080/health"
	} else {
		tenantHealthURL += "/health"
	}
	hc.Register("tenant-service",
		healthz.HTTPCheck(tenantHealthURL, 2*time.Second),
		healthz.Soft(),
		healthz.Timeout(2*time.Second))

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1", eventHandler.Routes())

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Запускаем relay в отдельной goroutine. Run() блокируется до ctx.Done().
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if rerr := relayBundle.Run(ctx); rerr != nil {
			log.Error("outbox relay вышел с ошибкой", "err", rerr)
		}
	}()

	go func() {
		log.Info("billing-service запускается", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("billing-service: graceful shutdown")
	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(shutCtx)

	// Дожидаемся завершения relay (он уже получил ctx.Done()).
	wg.Wait()
	log.Info("billing-service остановлен")
}

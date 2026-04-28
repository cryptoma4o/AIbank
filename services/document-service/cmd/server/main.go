package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"
	"github.com/aibank/platform/services/document-service/internal/audit"
	"github.com/aibank/platform/services/document-service/internal/handler"
	"github.com/aibank/platform/services/document-service/internal/repository"
	"github.com/aibank/platform/services/document-service/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "document-service",
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

	st, err := buildStorage(log)
	if err != nil {
		log.Error("init storage", "err", err)
		os.Exit(1)
	}

	docRepo := repository.NewPostgresDocumentRepository(db)
	auditClient := audit.MustClient(log)
	docHandler := handler.NewDocumentHandler(docRepo, st, auditClient, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("document-service"))
	r.Use(middleware.Timeout(60 * time.Second))

	// Структурированные probe-эндпоинты на packages/healthz.
	// Storage-бэкенд: для memory-режима используем AlwaysHealthy("memory"),
	// real-MinIO probe — TODO (нужен GET /minio/health/live или ListBuckets).
	hc := healthz.New("document-service", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))
	storageBackend := os.Getenv("STORAGE_BACKEND")
	if storageBackend == "" {
		storageBackend = "memory"
	}
	if storageBackend == "memory" {
		hc.Register("storage", healthz.AlwaysHealthy("memory"))
	} else {
		// TODO: реальная проверка MinIO (HTTP/health) — пока soft-degraded.
		hc.Register("storage", healthz.AlwaysHealthy("minio probe TODO"), healthz.Soft())
	}

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1/documents", docHandler.Routes())

	srv := &http.Server{
		Addr:         ":8083",
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("document-service starting", "addr", srv.Addr)
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

// buildStorage выбирает backend по STORAGE_BACKEND (memory|minio).
func buildStorage(log *slog.Logger) (storage.Storage, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "memory"
	}
	switch backend {
	case "memory":
		log.Info("storage backend selected", "backend", "memory")
		return storage.NewInMemoryStorage(), nil
	case "minio":
		useSSL := true
		if v := os.Getenv("STORAGE_MINIO_USE_SSL"); v != "" {
			parsed, err := strconv.ParseBool(v)
			if err == nil {
				useSSL = parsed
			}
		}
		cfg := storage.MinIOConfig{
			Endpoint:  os.Getenv("STORAGE_MINIO_ENDPOINT"),
			AccessKey: os.Getenv("STORAGE_MINIO_ACCESS_KEY"),
			SecretKey: os.Getenv("STORAGE_MINIO_SECRET_KEY"),
			Bucket:    os.Getenv("STORAGE_MINIO_BUCKET"),
			UseSSL:    useSSL,
		}
		log.Info("storage backend selected",
			"backend", "minio", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket)
		return storage.NewMinIOStorage(cfg)
	default:
		log.Warn("unknown STORAGE_BACKEND, falling back to memory", "value", backend)
		return storage.NewInMemoryStorage(), nil
	}
}

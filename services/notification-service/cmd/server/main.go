// Package main — notification-service HTTP-сервер (port 8110).
//
// ENV:
//
//	DATABASE_URL          — postgres connection string (если пусто, repo
//	                        использует in-memory заглушку для dev)
//	NOTIFICATION_SENDER   — mock|smtp|sms_stub (default mock)
//	SMTP_HOST/PORT/USER/PASS/FROM — для NOTIFICATION_SENDER=smtp
//	SMS_PROVIDER          — для NOTIFICATION_SENDER=sms_stub
//	PORT                  — по умолчанию 8110
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
	chimw "github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	obs "github.com/aibank/platform/packages/observability"
	"github.com/aibank/platform/services/notification-service/internal/domain"
	"github.com/aibank/platform/services/notification-service/internal/handler"
	"github.com/aibank/platform/services/notification-service/internal/repository"
	"github.com/aibank/platform/services/notification-service/internal/sender"
	"github.com/aibank/platform/services/notification-service/internal/templates"
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
		ServiceName: "notification-service",
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

	tpl, err := templates.NewRegistry()
	if err != nil {
		log.Error("init template registry", "err", err)
		os.Exit(1)
	}
	log.Info("templates loaded", "ids", tpl.IDs())

	repo, closeFn := buildRepo(log)
	defer closeFn()

	senders := buildSenders(log)

	h := handler.New(repo, tpl, senders, log)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(obs.ChiMiddleware("notification-service"))
	r.Use(chimw.Timeout(20 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	r.Mount("/v1", h.Routes())

	port := env("PORT", "8110")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("notification-service listening", "addr", srv.Addr)
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

// buildRepo — Postgres-репозиторий, если DATABASE_URL задан; иначе
// in-memory заглушка для dev.
func buildRepo(log *slog.Logger) (domain.NotificationRepository, func()) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Warn("DATABASE_URL is empty — using in-memory notification repo (dev mode)")
		return handler.NewInMemoryRepo(), func() {}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Error("ping db", "err", err)
		os.Exit(1)
	}
	return repository.NewPostgresNotificationRepository(db), func() { _ = db.Close() }
}

// buildSenders — фабрика sender-ов на основе NOTIFICATION_SENDER.
//
// mock      → MockSender для всех каналов
// smtp      → SMTPSender для email; mock для остального
// sms_stub  → SMSStubSender для sms; mock для остального
func buildSenders(log *slog.Logger) map[domain.RecipientType]sender.Sender {
	mode := env("NOTIFICATION_SENDER", "mock")
	out := map[domain.RecipientType]sender.Sender{
		domain.RecipientEmail: sender.NewMockSender(domain.RecipientEmail),
		domain.RecipientSMS:   sender.NewMockSender(domain.RecipientSMS),
		domain.RecipientPush:  sender.NewMockSender(domain.RecipientPush),
	}
	switch mode {
	case "mock":
		// already set
	case "smtp":
		port, _ := strconv.Atoi(env("SMTP_PORT", "587"))
		out[domain.RecipientEmail] = sender.NewSMTPSender(sender.SMTPConfig{
			Host:     env("SMTP_HOST", "localhost"),
			Port:     port,
			Username: os.Getenv("SMTP_USER"),
			Password: os.Getenv("SMTP_PASS"),
			From:     env("SMTP_FROM", "noreply@platform.ru"),
		})
	case "sms_stub":
		out[domain.RecipientSMS] = sender.NewSMSStubSender(env("SMS_PROVIDER", "stub"))
	default:
		log.Warn("unknown NOTIFICATION_SENDER, falling back to mock", "value", mode)
	}
	log.Info("senders configured", "mode", mode)
	return out
}

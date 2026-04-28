// Package main — точка входа адаптера Diasoft FA# (см. ADR-0006).
//
// Сервис слушает HTTP на :9002 и предоставляет:
//
//   - GET  /healthz     — k8s readiness/liveness probe
//   - GET  /version     — adapter+version+commands (упрощённый Capabilities)
//   - POST /v1/execute  — канонический транспорт abs-connector ↔ adapter
//
// Версия читается из embedded VERSION-файла (`go:embed`) корневым
// пакетом adapterdiasoft. Это нужно, чтобы Docker-image-tag
// (`aibank/abs-adapter-diasoft:<version>`) и значение в audit log
// оставались согласованными даже при rebuild без правки кода.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	obs "github.com/aibank/platform/packages/observability"

	adapterdiasoft "aibank/abs-adapter-diasoft"
	"aibank/abs-adapter-diasoft/internal/handler"
)

func main() {
	version := strings.TrimSpace(adapterdiasoft.Version())
	if version == "" {
		version = "0.0.0-dev"
	}

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md). Чейн-роутер собирается
	// внутри handler.Handler.Router() — оборачиваем верхнеуровневый
	// http.Handler через otelhttp.NewHandler ниже, чтобы получить серверные
	// span'ы без модификации handler-логики.
	obsLog := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "abs-adapter-diasoft",
		Version:     os.Getenv("OTEL_SERVICE_VERSION"),
		Environment: os.Getenv("DEPLOY_ENV"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Logger:      obsLog,
	})
	obsCancel()
	if err != nil {
		log.Fatalf("abs-adapter-diasoft: init observability: %v", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = obsProvider.Shutdown(shutCtx)
	}()

	h := handler.NewHandler(version)

	srv := &http.Server{
		Addr:         ":9002",
		Handler:      otelhttp.NewHandler(h.Router(), "abs-adapter-diasoft"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("abs-adapter-diasoft %s: listening on :9002", version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("abs-adapter-diasoft: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("abs-adapter-diasoft: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("abs-adapter-diasoft: forced shutdown: %v", err)
	}
	log.Println("abs-adapter-diasoft: stopped")
}

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	obs "github.com/aibank/platform/packages/observability"

	"aibank/ext-egrul/internal/cache"
	"aibank/ext-egrul/internal/handler"
	"aibank/ext-egrul/internal/provider"
)

// ext-egrul — сервис интеграции с ФНС ЕГРЮЛ/ЕГРИП.
//
// На текущем этапе реальный API ФНС не вызывается, ответы синтезируются
// детерминированно по входному ИНН/ОГРН. См. internal/domain/stubs.go.
//
// Конфигурация через переменные окружения:
//
//	REDIS_ADDR        — адрес Redis (default redis:6379)
//	CACHE_TTL_HOURS   — TTL кэша в часах (default 24)
//	PORT              — порт HTTP (default 8201)
//	EGRUL_LIVE        — true для реального ФНС-клиента (default: false → synthetic)
//	EGRUL_LIVE_*      — параметры live-режима (см. internal/provider/factory.go)
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md). Чейн-роутер собирается
	// внутри handler.NewHandler — оборачиваем верхнеуровневый http.Handler
	// через otelhttp.NewHandler ниже, чтобы получить серверные span'ы без
	// модификации handler-логики.
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "ext-egrul",
		Version:     os.Getenv("OTEL_SERVICE_VERSION"),
		Environment: os.Getenv("DEPLOY_ENV"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Logger:      logger,
	})
	obsCancel()
	if err != nil {
		logger.Error("ext-egrul: init observability", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = obsProvider.Shutdown(shutCtx)
	}()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8201"
	}

	redisCache := cache.NewRedisCache(redisAddr)
	logger.Info("ext-egrul: cache ready", "redis_addr", redisAddr, "ttl", redisCache.TTL().String())

	prov := provider.BuildProvider()
	logger.Info("ext-egrul: provider выбран", "provider", prov.Name())

	h := handler.NewHandler(redisCache, prov)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      otelhttp.NewHandler(h, "ext-egrul"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("ext-egrul: HTTP сервер стартует", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("ext-egrul: ошибка сервера", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("ext-egrul: останавливаемся")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("ext-egrul: graceful shutdown failed", "err", err)
	}
	logger.Info("ext-egrul: остановлен")
}

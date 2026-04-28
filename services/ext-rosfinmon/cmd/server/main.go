package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/ext-rosfinmon/internal/cache"
	"aibank/ext-rosfinmon/internal/handler"
	"aibank/ext-rosfinmon/internal/provider"
)

// ext-rosfinmon — сервис проверки субъектов по перечню 115-ФЗ.
//
// Реальный feed Росфинмониторинга не используется — отдаём синтетику.
//
// Конфигурация:
//
//	REDIS_ADDR             — адрес Redis (default redis:6379)
//	SCREENING_TTL_HOURS    — TTL результатов скрининга (default 168 = 7 дней)
//	SNAPSHOT_TTL_HOURS     — TTL снапшота списка (default 24)
//	PORT                   — порт HTTP (default 8202)
//	RFM_LIVE               — true для реального ФСФМ-клиента (default: false → synthetic)
//	RFM_LIVE_*             — параметры live-режима (см. internal/provider/factory.go)
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = os.Getenv("REDIS_URL")
	}
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8202"
	}

	rc := cache.NewRedisCache(redisAddr)
	logger.Info("ext-rosfinmon: cache ready",
		"redis_addr", redisAddr,
		"screening_ttl", rc.ScreeningTTL().String(),
		"snapshot_ttl", rc.SnapshotTTL().String(),
	)

	prov := provider.BuildProvider()
	logger.Info("ext-rosfinmon: provider выбран", "provider", prov.Name())

	h := handler.NewHandler(rc, prov)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("ext-rosfinmon: HTTP сервер стартует", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("ext-rosfinmon: ошибка сервера", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("ext-rosfinmon: останавливаемся")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("ext-rosfinmon: graceful shutdown failed", "err", err)
	}
	logger.Info("ext-rosfinmon: остановлен")
}

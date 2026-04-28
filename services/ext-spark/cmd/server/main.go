package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/ext-spark/internal/cache"
	"aibank/ext-spark/internal/handler"
	"aibank/ext-spark/internal/provider"
)

// ext-spark — опциональный сервис интеграции со СПАРК / Контур.Фокус.
//
// Реальный API провайдера не вызывается — детерминированная синтетика по ИНН.
// Этот сервис помечен как «легкий» / supplementary в § 4.3 техдокумента.
//
// Конфигурация:
//
//	REDIS_ADDR        — адрес Redis (default redis:6379)
//	CACHE_TTL_HOURS   — TTL кэша (default 24)
//	PORT              — порт HTTP (default 8204)
//	SPARK_LIVE        — true для реального клиента СПАРК/Фокус (default: false → synthetic)
//	SPARK_LIVE_*      — параметры live-режима (см. internal/provider/factory.go)
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8204"
	}

	rc := cache.NewRedisCache(redisAddr)
	logger.Info("ext-spark: cache ready", "redis_addr", redisAddr, "ttl", rc.TTL().String())

	prov := provider.BuildProvider()
	logger.Info("ext-spark: provider выбран", "provider", prov.Name())

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
		logger.Info("ext-spark: HTTP сервер стартует", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("ext-spark: ошибка сервера", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("ext-spark: останавливаемся")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("ext-spark: graceful shutdown failed", "err", err)
	}
	logger.Info("ext-spark: остановлен")
}

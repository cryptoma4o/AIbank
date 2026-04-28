package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/ext-fssp/internal/cache"
	"aibank/ext-fssp/internal/handler"
	"aibank/ext-fssp/internal/provider"
)

// ext-fssp — сервис проверки исполнительных производств ФССП.
//
// Реальный API ФССП (OpenData) не вызывается — детерминированная синтетика по
// ИНН / (ФИО + дата рождения).
//
// Конфигурация:
//
//	REDIS_ADDR        — адрес Redis (default redis:6379)
//	CACHE_TTL_HOURS   — TTL кэша (default 24)
//	PORT              — порт HTTP (default 8203)
//	FSSP_LIVE         — true для реального ФССП-клиента (default: false → synthetic)
//	FSSP_LIVE_*       — параметры live-режима (см. internal/provider/factory.go)
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8203"
	}

	rc := cache.NewRedisCache(redisAddr)
	logger.Info("ext-fssp: cache ready", "redis_addr", redisAddr, "ttl", rc.TTL().String())

	prov := provider.BuildProvider()
	logger.Info("ext-fssp: provider выбран", "provider", prov.Name())

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
		logger.Info("ext-fssp: HTTP сервер стартует", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("ext-fssp: ошибка сервера", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("ext-fssp: останавливаемся")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("ext-fssp: graceful shutdown failed", "err", err)
	}
	logger.Info("ext-fssp: остановлен")
}

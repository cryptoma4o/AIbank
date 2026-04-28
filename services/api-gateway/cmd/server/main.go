// Package main — api-gateway HTTP-сервер (port 8000).
//
// Чейн middleware:
//
//	RequestID → RealIP → Recoverer → TenantResolver → AccessLog →
//	RateLimit (per tenant+ip) → optional Auth → Proxy
//
// ENV:
//
//	BFF_ONBOARDING_URL   — http://bff-onboarding:8091
//	BFF_ADMIN_URL        — http://bff-admin:8092
//	LLM_GATEWAY_URL      — http://llm-gateway:8100  (mock-режим допустим)
//	AUDIT_SERVICE_URL    — http://audit-service:8081
//	JWT_SECRET           — общий с identity-service (>=32 байт, REQUIRED)
//	REDIS_URL            — redis://host:6379/0  (или пусто → in-memory)
//	PLATFORM_DOMAINS     — csv доменов (default: platform.ru,localhost)
//	PUBLIC_ROUTES        — csv URL-префиксов без auth (default:
//	                       /api/onboarding/v1/auth/login,/api/onboarding/v1/applicants)
//	AUTH_DEV_MODE        — "1" чтобы разрешить пустой JWT_SECRET (только dev)
//	PORT                 — по умолчанию 8000
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
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	obs "github.com/aibank/platform/packages/observability"

	"aibank/api-gateway/internal/audit"
	gwauth "aibank/api-gateway/internal/auth"
	"aibank/api-gateway/internal/proxy"
	"aibank/api-gateway/internal/ratelimit"
	"aibank/api-gateway/internal/router"
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
		ServiceName: "api-gateway",
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

	cfg := proxy.Config{
		BFFOnboardingURL: env("BFF_ONBOARDING_URL", "http://bff-onboarding:8091"),
		BFFAdminURL:      env("BFF_ADMIN_URL", "http://bff-admin:8092"),
		LLMGatewayURL:    env("LLM_GATEWAY_URL", "http://llm-gateway:8100"),
		AuditServiceURL:  env("AUDIT_SERVICE_URL", "http://audit-service:8081"),
	}
	proxyRouter, err := proxy.NewRouter(cfg)
	if err != nil {
		log.Error("build proxy router", "err", err)
		os.Exit(1)
	}

	// Tenant resolver
	suffixes := DefaultSuffixesFromEnv()
	tenantResolver := router.NewResolver(suffixes)

	// Rate limiter: 60 rpm + burst 10 per (tenant,ip)
	limiterStore := buildLimiterStore(log)
	limiter := ratelimit.New(limiterStore, ratelimit.Config{RatePerMinute: 60, Burst: 10})

	// JWT verifier — REQUIRED в production; AUTH_DEV_MODE=1 включает legacy
	// no-auth chain (только для локальной разработки).
	authMW := buildAuthMiddleware(log)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(obs.ChiMiddleware("api-gateway"))
	r.Use(chimw.Timeout(30 * time.Second))

	// Health/ready — без tenant-resolution и без auth.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// Защищённые /api/* — полный middleware-стек.
	r.Group(func(g chi.Router) {
		g.Use(tenantResolver.Middleware)
		g.Use(audit.AccessLog(log))
		g.Use(limiter.Middleware)
		g.Use(authMW)
		g.Mount("/api", http.HandlerFunc(proxyRouter.ServeHTTP))
	})

	port := env("PORT", "8000")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("api-gateway listening", "addr", srv.Addr,
			"targets", proxyRouter.Targets())
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

// DefaultSuffixesFromEnv — суффиксы платформы из PLATFORM_DOMAINS (csv).
func DefaultSuffixesFromEnv() []string {
	v := os.Getenv("PLATFORM_DOMAINS")
	if v == "" {
		return router.DefaultSuffixes
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return router.DefaultSuffixes
	}
	return out
}

// buildAuthMiddleware — собирает auth-middleware с учётом env.
//
//   - JWT_SECRET выставлен → строгая валидация: 401 на отсутствие токена
//     везде, кроме PUBLIC_ROUTES; identity-headers ставятся из claim'ов.
//   - JWT_SECRET пустой и AUTH_DEV_MODE=1 → middleware-no-op (dev only,
//     downstream НЕ получают X-Tenant-ID/X-Actor-ID — это видно по логам
//     сервисов и ловится в QA).
//   - JWT_SECRET пустой и AUTH_DEV_MODE≠1 → fail-fast.
func buildAuthMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if strings.EqualFold(os.Getenv("AUTH_DEV_MODE"), "1") ||
			strings.EqualFold(os.Getenv("AUTH_DEV_MODE"), "true") {
			log.Warn("JWT_SECRET is empty and AUTH_DEV_MODE=1 — auth disabled (dev only!)")
			return func(next http.Handler) http.Handler { return next }
		}
		log.Error("JWT_SECRET is required (>=32 bytes); set AUTH_DEV_MODE=1 to bypass for local dev")
		os.Exit(1)
	}
	v, err := gwauth.NewVerifier([]byte(secret))
	if err != nil {
		log.Error("init JWT verifier", "err", err)
		os.Exit(1)
	}

	publicRoutes := gwauth.ParsePublicRoutes(env("PUBLIC_ROUTES",
		"/api/onboarding/v1/auth/login,/api/onboarding/v1/applicants"))
	log.Info("auth middleware enabled",
		"public_routes", publicRoutes,
		"enforce_tenant_match", true)

	return gwauth.Middleware(v, gwauth.Options{
		Optional:           false,
		EnforceTenantMatch: true,
		PublicRoutes:       publicRoutes,
	})
}

// buildLimiterStore — Redis если есть REDIS_URL, иначе in-memory.
func buildLimiterStore(log *slog.Logger) ratelimit.Store {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Warn("REDIS_URL is empty — using in-memory rate limit store (dev mode)")
		return ratelimit.NewInMemoryStore()
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		log.Error("parse REDIS_URL", "err", err)
		os.Exit(1)
	}
	c := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		log.Warn("redis ping failed, falling back to in-memory", "err", err)
		return ratelimit.NewInMemoryStore()
	}
	return &ratelimit.RedisStore{Client: c}
}

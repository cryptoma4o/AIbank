// Package main — точка входа abs-connector.
//
// Связывает AdapterRegistry (configs/adapters.yaml + env override),
// IdempotencyStore (Redis или in-memory) и chi-handler в один HTTP-сервер.
// Порт по умолчанию — :8088 (см. ТЗ задачи; docker-compose не модифицируем).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"

	"aibank/abs-connector/internal/clients"
	"aibank/abs-connector/internal/dedupstore"
	"aibank/abs-connector/internal/domain"
	"aibank/abs-connector/internal/handler"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if code := run(log); code != 0 {
		os.Exit(code)
	}
}

func run(log *slog.Logger) int {
	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md). Чейн-роутер в abs-connector
	// собирается внутри handler.Handler.Router(); поэтому ChiMiddleware подключить
	// нельзя, не трогая handler-код. Вместо неё оборачиваем верхнеуровневый
	// http.Handler через otelhttp.NewHandler ниже — это даёт серверные
	// span'ы без модификации handler-логики.
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "abs-connector",
		Version:     os.Getenv("OTEL_SERVICE_VERSION"),
		Environment: os.Getenv("DEPLOY_ENV"),
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Logger:      log,
	})
	obsCancel()
	if err != nil {
		log.Error("init observability", "err", err)
		return 1
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = obsProvider.Shutdown(shutCtx)
	}()

	registry := domain.NewAdapterRegistry()

	// 1. Конфиг-файл (configs/adapters.yaml).
	cfgPath := os.Getenv("ABS_ADAPTERS_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/adapters.yaml"
	}
	if _, err := os.Stat(cfgPath); err == nil {
		if err := registry.LoadFromYAMLFile(cfgPath); err != nil {
			log.Error("load adapters yaml failed", "path", cfgPath, "err", err)
			return 1
		}
		log.Info("adapters yaml loaded", "path", cfgPath, "count", len(registry.All()))
	} else if !os.IsNotExist(err) {
		log.Error("stat adapters yaml failed", "path", cfgPath, "err", err)
		return 1
	} else {
		log.Warn("adapters yaml not found, relying on env overrides", "path", cfgPath)
	}

	// 2. Env-overrides (ABS_ADAPTER_<TENANTID>=name,url,version).
	if err := registry.LoadFromEnv(os.Environ()); err != nil {
		log.Error("load env adapters failed", "err", err)
		return 1
	}

	if len(registry.All()) == 0 {
		log.Warn("adapter registry is empty; all /v1/commands will return 503")
	}

	// 3. IdempotencyStore: Redis по умолчанию, in-memory как опция.
	storeKind := strings.ToLower(strings.TrimSpace(os.Getenv("DEDUP_STORE")))
	if storeKind == "" {
		storeKind = "redis"
	}

	var (
		store     domain.IdempotencyStore
		readyCheck func(ctx context.Context) error
		rdb       *redis.Client
	)

	switch storeKind {
	case "memory", "in-memory", "inmemory":
		store = dedupstore.NewInMemoryStore()
		readyCheck = func(context.Context) error { return nil }
		log.Info("idempotency store: in-memory")
	case "redis":
		addr := firstNonEmpty(os.Getenv("REDIS_URL"), os.Getenv("REDIS_ADDR"), "redis:6379")
		opts, err := parseRedisAddr(addr)
		if err != nil {
			log.Error("parse REDIS_URL failed", "addr", addr, "err", err)
			return 1
		}
		rdb = redis.NewClient(opts)
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			pingCancel()
			log.Error("redis ping failed", "addr", addr, "err", err)
			return 1
		}
		pingCancel()
		store = dedupstore.NewRedisStore(rdb, dedupstore.DefaultTTL)
		readyCheck = func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
		log.Info("idempotency store: redis", "addr", addr)
	default:
		log.Error("unknown DEDUP_STORE", "value", storeKind)
		return 1
	}

	client := clients.NewAdapterClient(clients.DefaultRequestTimeout)

	h := handler.New(handler.Config{
		Registry:  registry,
		Store:     store,
		Client:    client,
		CacheTTL:  dedupstore.DefaultTTL,
		Logger:    log,
		ReadReady: readyCheck,
	})

	// Структурированные probe-эндпоинты на packages/healthz.
	// abs-connector ранее регистрировал /health и /ready в handler.Router();
	// здесь оборачиваем его внешним chi-роутером, который перехватывает
	// /health и /ready и делегирует их healthz.HealthChecker'у. Старые
	// маршруты внутри handler.Router() становятся unreachable (внешний
	// роутер матчится первым), но handler-код мы при этом не трогаем.
	hc := healthz.New("abs-connector", os.Getenv("OTEL_SERVICE_VERSION"))
	if rdb != nil {
		hc.Register("redis",
			healthz.FuncCheck(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
			healthz.Timeout(2*time.Second))
	}
	// Soft HTTP-проверка по первому сконфигурированному адаптеру.
	// Если адаптеров нет — readiness не блокируется.
	if all := registry.All(); len(all) > 0 {
		hc.Register("adapter",
			healthz.HTTPCheck(all[0].URL, 2*time.Second),
			healthz.Soft(),
			healthz.Timeout(2*time.Second))
	}

	outer := chi.NewRouter()
	outer.Method(http.MethodGet, "/health", hc.LivenessHandler())
	outer.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	outer.Mount("/", h.Router())

	port := firstNonEmpty(os.Getenv("PORT"), "8088")
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      otelhttp.NewHandler(outer, "abs-connector"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("abs-connector starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("abs-connector shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown failed", "err", err)
	}
	if rdb != nil {
		_ = rdb.Close()
	}
	log.Info("abs-connector stopped")
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// parseRedisAddr принимает как «host:port», так и redis://-URL.
func parseRedisAddr(addr string) (*redis.Options, error) {
	if strings.Contains(addr, "://") {
		return redis.ParseURL(addr)
	}
	return &redis.Options{Addr: addr}, nil
}

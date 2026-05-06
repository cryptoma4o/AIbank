package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"
	"github.com/aibank/platform/packages/secrets"

	"aibank/identity-service/internal/audit"
	"aibank/identity-service/internal/auth"
	"aibank/identity-service/internal/handler"
	"aibank/identity-service/internal/repository"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "identity-service",
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

	// Secrets are resolved via packages/secrets (Vault in prod, env in dev).
	// Direct env vars still win for backward-compat / dev workflow,
	// per docs/security-architecture.md § 7.3 + ADR-0002.
	//
	// Env contract:
	//   DATABASE_URL / JWT_SECRET    direct values (dev shortcut, take precedence)
	//   DATABASE_URL_SECRET_KEY      provider key, default "database/url"
	//   JWT_SECRET_KEY               provider key, default "jwt/secret"
	//   SECRETS_BACKEND              env | vault | chained (default chained)
	//   VAULT_ADDR / VAULT_TOKEN     Vault wiring (see packages/secrets/README.md)
	provider, err := secrets.BuildProvider(secrets.FactoryConfig{
		ServiceName: "identity-service",
	})
	if err != nil {
		log.Error("init secrets provider", "err", err)
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		key := os.Getenv("DATABASE_URL_SECRET_KEY")
		if key == "" {
			key = "database/url"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		dsn, err = provider.GetSecret(ctx, key)
		cancel()
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				log.Error("DATABASE_URL not configured (env or secrets backend)", "key", key)
			} else {
				log.Error("fetch DATABASE_URL secret", "err", err, "key", key)
			}
			os.Exit(1)
		}
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		key := os.Getenv("JWT_SECRET_KEY")
		if key == "" {
			key = "jwt/secret"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		jwtSecret, err = provider.GetSecret(ctx, key)
		cancel()
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				log.Error("JWT_SECRET not configured (env or secrets backend)", "key", key)
			} else {
				log.Error("fetch JWT_SECRET secret", "err", err, "key", key)
			}
			os.Exit(1)
		}
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

	issuer, err := auth.NewIssuer([]byte(jwtSecret))
	if err != nil {
		log.Error("init jwt issuer", "err", err)
		os.Exit(1)
	}
	verifier, err := auth.NewVerifier([]byte(jwtSecret))
	if err != nil {
		log.Error("init jwt verifier", "err", err)
		os.Exit(1)
	}

	users := repository.NewPostgresUserRepository(db)
	// Field-level encryption (Vault Transit) wiring — TODO: подключить
	// secrets.NewTransitClient под флагом VAULT_TRANSIT_ENABLED. Сейчас
	// applicants пишут plaintext в TEXT-колонки (BYTEA *_enc остаются NULL).
	applicants := repository.NewPostgresApplicantRepository(db, nil)
	consents := repository.NewPostgresConsentRepository(db)
	auditClient := audit.MustClient(log)
	authHandler := handler.NewAuthHandler(users, applicants, consents, issuer, verifier, auditClient, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("identity-service"))
	r.Use(middleware.Timeout(30 * time.Second))
	// CORS для browser-side fetch из web-admin / web-onboarding.
	// CORS_ALLOWED_ORIGINS — comma-separated list, либо "*" для дева.
	// На staging задаётся в docker-compose.override.yml.
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   strings.Split(origins, ","),
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Структурированные probe-эндпоинты на packages/healthz.
	hc := healthz.New("identity-service", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1", authHandler.Routes())

	srv := &http.Server{
		Addr:         ":8082",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("identity-service starting", "addr", srv.Addr)
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

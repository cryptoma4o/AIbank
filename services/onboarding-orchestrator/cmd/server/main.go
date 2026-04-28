// Command server запускает онбординг-оркестратор: HTTP API + Temporal worker.
//
// HTTP — порт 8085 (env PORT).  Temporal — env TEMPORAL_HOST (по умолчанию
// localhost:7233), TEMPORAL_NAMESPACE (по умолчанию default).
//
// Activity-стабы возвращают errors.New("real activity not wired") — этого
// достаточно, чтобы worker зарегистрировался, но прод-флоу не сработали.
// Реальные активности живут в отдельных сервисах (identity, document,
// reconciliation, risk, abs) и подключаются через DI на этапе deploy.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"

	"aibank/onboarding-orchestrator/internal/audit"
	"aibank/onboarding-orchestrator/internal/handler"
	"aibank/onboarding-orchestrator/internal/repository"
	wf "aibank/onboarding-orchestrator/internal/workflow"
)

// uuidGenerator реализует handler.IDGenerator через google/uuid.
type uuidGenerator struct{}

func (uuidGenerator) NewApplicationID() string { return "app_" + uuid.NewString() }

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "onboarding-orchestrator",
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
	temporalHost := getenv("TEMPORAL_HOST", "localhost:7233")
	temporalNS := getenv("TEMPORAL_NAMESPACE", "default")
	httpAddr := ":" + getenv("PORT", "8085")

	// ── DB ────────────────────────────────────────────────────────────
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

	// ── Temporal ──────────────────────────────────────────────────────
	tc, err := client.Dial(client.Options{
		HostPort:  temporalHost,
		Namespace: temporalNS,
	})
	if err != nil {
		log.Error("temporal dial", "host", temporalHost, "err", err)
		os.Exit(1)
	}
	defer tc.Close()

	w := worker.New(tc, wf.TaskQueue, worker.Options{})
	w.RegisterWorkflow(wf.OnboardingWorkflow)
	registerActivityStubs(w)

	if err := w.Start(); err != nil {
		log.Error("worker start", "err", err)
		os.Exit(1)
	}
	defer w.Stop()

	// ── HTTP ──────────────────────────────────────────────────────────
	repo := repository.NewPostgresApplicationRepository(db)
	auditClient := audit.MustClient(log)
	appHandler := handler.NewApplicationHandler(repo, tc, uuidGenerator{}, auditClient, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("onboarding-orchestrator"))
	r.Use(middleware.Timeout(30 * time.Second))

	// Структурированные probe-эндпоинты на packages/healthz.
	// Temporal проверяется TCP-dial'ом по host:port — frontend grpc-сервис
	// слушает порт 7233 и для readiness достаточно убедиться, что listener жив.
	hc := healthz.New("onboarding-orchestrator", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))
	hc.Register("temporal", healthz.TCPCheck(temporalHost, 2*time.Second), healthz.Timeout(2*time.Second))

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1/applications", appHandler.Routes())

	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("onboarding-orchestrator starting",
			"addr", srv.Addr,
			"temporal", temporalHost,
			"namespace", temporalNS)
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

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// registerActivityStubs регистрирует STUB-имплементации активностей, чтобы
// worker мог запуститься, но любой реальный workflow-flow упирался в
// явную ошибку.  Реальные активности подключаются в отдельных деплоях
// (identity-service, document-service, reconciliation-service, risk-engine,
// abs-connector) — каждый из них регистрирует свой набор активностей в
// той же task queue под теми же именами.
//
// TODO: вынести регистрацию активностей в DI-обвязку, когда появятся
// реальные клиенты внешних сервисов (см. plan: services/identity-service,
// services/document-service, services/reconciliation-service,
// services/risk-engine, services/abs-connector).
func registerActivityStubs(w worker.Worker) {
	stub := func(ctx context.Context, _ ...any) (any, error) {
		activity.GetLogger(ctx).Warn("activity stub invoked — real activity not wired")
		return nil, errors.New("real activity not wired")
	}
	for _, name := range []string{
		wf.ActivityVerifyIdentity,
		wf.ActivityExtractDocument,
		wf.ActivityReconcile,
		wf.ActivityAssessRisk,
		wf.ActivityOpenAccount,
		wf.ActivityCancelAccount,
	} {
		w.RegisterActivityWithOptions(stub, activity.RegisterOptions{Name: name})
	}
}

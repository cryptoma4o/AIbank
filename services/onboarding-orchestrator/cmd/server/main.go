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
	temporalactivity "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"

	"aibank/onboarding-orchestrator/internal/activity"
	"aibank/onboarding-orchestrator/internal/audit"
	"aibank/onboarding-orchestrator/internal/extclients"
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

	// Audit client first — activities and HTTP handlers share it.
	auditClient := audit.MustClient(log)

	w := worker.New(tc, wf.TaskQueue, worker.Options{})
	w.RegisterWorkflow(wf.OnboardingWorkflow)
	registerActivities(w, auditClient, log)

	if err := w.Start(); err != nil {
		log.Error("worker start", "err", err)
		os.Exit(1)
	}
	defer w.Stop()

	// ── HTTP ──────────────────────────────────────────────────────────
	repo := repository.NewPostgresApplicationRepository(db)
	profileRepo := repository.NewPostgresProfileRepository(db)
	activityRepo := repository.NewPostgresActivityRepository(db)
	repRepo := repository.NewPostgresRepresentativeRepository(db)
	uboRepo := repository.NewPostgresUBOGraphRepository(db)
	screeningRepo := repository.NewPostgresScreeningRepository(db)
	monitoringRepo := repository.NewPostgresMonitoringRepository(db)
	accountRepo := repository.NewPostgresAccountRepository(db)
	appHandler := handler.NewApplicationHandler(repo, tc, uuidGenerator{}, auditClient, log)
	profileHandler := handler.NewProfileHandler(profileRepo, log)
	activityHandler := handler.NewActivityHandler(activityRepo, log)
	repHandler := handler.NewRepresentativeHandler(repRepo, log)
	uboHandler := handler.NewUBOHandler(uboRepo, log)
	screeningHandler := handler.NewScreeningHandler(screeningRepo, log)
	monitoringHandler := handler.NewMonitoringHandler(monitoringRepo, log)
	accountHandler := handler.NewAccountHandler(accountRepo, log)

	// Этап 1 формы онбординга — параллельный скоринг по ИНН/ОГРН через
	// ext-egrul / ext-rosfinmon / ext-fssp. URLs feature-флаговые: пустая
	// переменная = источник недоступен, handler пометит его как unavailable.
	extc := extclients.New(
		os.Getenv("EXT_EGRUL_URL"),
		os.Getenv("EXT_ROSFINMON_URL"),
		os.Getenv("EXT_FSSP_URL"),
	)
	prequalifyHandler := handler.NewPrequalifyHandler(extc, log)

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
	r.Post("/v1/prequalify", prequalifyHandler.Prequalify)
	r.Mount("/v1/legal-entity-profiles", profileHandler.Routes())
	r.Mount("/v1/application-activities", activityHandler.Routes())
	r.Mount("/v1/representatives", repHandler.Routes())
	r.Mount("/v1/ubo-graphs", uboHandler.Routes())
	r.Mount("/v1/screenings", screeningHandler.Routes())
	r.Mount("/v1/monitoring-profiles", monitoringHandler.Routes())
	r.Mount("/v1/accounts", accountHandler.Routes())
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

// registerActivities wires Temporal activities into the worker.
//
// VerifyIdentity (→ identity-service) and AssessRisk (→ risk-engine) have real
// implementations in internal/activity. Их endpoint URLs приходят из env:
//   - IDENTITY_SERVICE_URL (по умолчанию http://identity-service:8082)
//   - RISK_ENGINE_URL      (по умолчанию http://risk-engine:8086)
// Empty env vars не блокируют worker — клиент сразу вернёт ошибку первого
// http-вызова, Temporal зафиксирует это в activity history.
//
// Остальные активности (ExtractDocument, Reconcile, OpenAccount, CancelAccount)
// пока что регистрируются стабами — соответствующие downstream-сервисы
// (document-service, reconciliation-service, abs-connector) ещё не имеют
// production-готовых контрактов. После их выкатки заменить стабы по аналогии.
func registerActivities(w worker.Worker, auditClient *auditsdk.Client, log *slog.Logger) {
	identityActs := activity.NewIdentityActivities(activity.IdentityConfig{
		BaseURL: getenv("IDENTITY_SERVICE_URL", "http://identity-service:8082"),
	}, auditClient, log)
	w.RegisterActivityWithOptions(identityActs.VerifyIdentity, temporalactivity.RegisterOptions{Name: wf.ActivityVerifyIdentity})

	riskActs := activity.NewRiskActivities(activity.RiskConfig{
		BaseURL: getenv("RISK_ENGINE_URL", "http://risk-engine:8086"),
	}, auditClient, log)
	w.RegisterActivityWithOptions(riskActs.RunRiskAssessment, temporalactivity.RegisterOptions{Name: wf.ActivityAssessRisk})

	stub := func(ctx context.Context, _ ...any) (any, error) {
		temporalactivity.GetLogger(ctx).Warn("activity stub invoked — real activity not wired")
		return nil, errors.New("real activity not wired")
	}
	for _, name := range []string{
		wf.ActivityExtractDocument,
		wf.ActivityReconcile,
		wf.ActivityOpenAccount,
		wf.ActivityCancelAccount,
	} {
		w.RegisterActivityWithOptions(stub, temporalactivity.RegisterOptions{Name: name})
	}
}

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

	ruleengine "aibank/rule-engine"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/healthz"
	obs "github.com/aibank/platform/packages/observability"
	"github.com/aibank/platform/services/risk-engine/internal/domain"
	"github.com/aibank/platform/services/risk-engine/internal/handler"
	"github.com/aibank/platform/services/risk-engine/internal/pipeline"
	"github.com/aibank/platform/services/risk-engine/internal/repository"
	"github.com/aibank/platform/services/risk-engine/internal/scorer"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// OpenTelemetry: traces + metrics через packages/observability.
	// При пустом OTEL_EXPORTER_OTLP_ENDPOINT провайдер запускается в no-op
	// режиме (см. packages/observability/README.md).
	obsCtx, obsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	obsProvider, err := obs.Init(obsCtx, obs.Config{
		ServiceName: "risk-engine",
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

	rulesPath := os.Getenv("RULES_PATH")
	if rulesPath == "" {
		rulesPath = "rules/default.yaml"
	}

	var rulesEngine *ruleengine.Engine
	if eng, err := ruleengine.NewEngineFromFile(rulesPath); err != nil {
		log.Warn("rules pack not loaded — pipeline will run without declarative rules",
			"path", rulesPath, "err", err)
	} else {
		rulesEngine = eng
		log.Info("rules pack loaded", "path", rulesPath)
	}

	repo := repository.NewPostgresRiskAssessmentRepository(db)
	// Backend selected via SCORER env (stub|onnx, default stub).
	// onnx требует RISK_MODEL_PATH=<file>.onnx; при init-error падает на stub.
	stubScorer := scorer.BuildScorer(log)
	textExplainer := scorer.NewTextExplainer()

	pipe := pipeline.NewPipeline(pipeline.Config{
		Extractor: domain.NewMapExtractor(),
		Scorer:    stubScorer,
		Explainer: textExplainer,
		Repo:      repo,
		Rules:     rulesEngine,
	})

	assessHandler := handler.NewAssessmentHandler(pipe, repo, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(obs.ChiMiddleware("risk-engine"))
	r.Use(middleware.Timeout(30 * time.Second))

	// Структурированные probe-эндпоинты на packages/healthz.
	// Rules-pack — soft check: если файла нет, движок работает в degraded-режиме
	// (без декларативных правил), но это не блокер для readiness.
	hc := healthz.New("risk-engine", os.Getenv("OTEL_SERVICE_VERSION"))
	hc.Register("db", healthz.DBCheck(db), healthz.Timeout(2*time.Second))
	hc.Register("rules", healthz.FileCheck(rulesPath), healthz.Soft())

	r.Method(http.MethodGet, "/health", hc.LivenessHandler())
	r.Method(http.MethodGet, "/ready", hc.HTTPHandler())
	r.Mount("/v1/assessments", assessHandler.Routes())

	srv := &http.Server{
		Addr:         ":8086",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("risk-engine starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

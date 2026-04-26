package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/risk-engine/internal/handler"
	"aibank/risk-engine/internal/scorer"
)

func main() {
	rulesPath := os.Getenv("RULES_PATH")
	if rulesPath == "" {
		rulesPath = "rules/default.yaml"
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ruleScorer, err := scorer.NewRuleScorerFromFile(rulesPath)
	if err != nil {
		log.Error("failed to load rules", "path", rulesPath, "error", err)
		os.Exit(1)
	}
	log.Info("rules loaded", "path", rulesPath)

	mlScorer := scorer.NewMLScorer()

	h := handler.New(ruleScorer, mlScorer)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Post("/v1/score", h.Score)

	srv := &http.Server{
		Addr:         ":8085",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("starting risk-engine", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	log.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", "error", err)
		os.Exit(1)
	}
	log.Info("server stopped")
}

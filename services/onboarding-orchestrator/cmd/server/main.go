package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/onboarding-orchestrator/internal/handler"
	"aibank/onboarding-orchestrator/internal/workflow"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const (
	taskQueue  = "onboarding"
	httpAddr   = ":8084"
)

func main() {
	temporalAddr := os.Getenv("TEMPORAL_ADDRESS")
	if temporalAddr == "" {
		temporalAddr = "temporal:7233"
	}

	// Connect to Temporal.
	tc, err := client.Dial(client.Options{HostPort: temporalAddr})
	if err != nil {
		log.Fatalf("failed to connect to Temporal at %s: %v", temporalAddr, err)
	}
	defer tc.Close()

	// Register workflow and activities on the worker.
	w := worker.New(tc, taskQueue, worker.Options{})
	w.RegisterWorkflow(workflow.OnboardingWorkflow)
	w.RegisterActivity(&workflow.Activities{})

	if err := w.Start(); err != nil {
		log.Fatalf("failed to start Temporal worker: %v", err)
	}
	defer w.Stop()

	// Build and start the HTTP server.
	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      handler.New(tc),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("HTTP server listening on %s", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server exited")
}

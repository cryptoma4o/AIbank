package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/bff-onboarding/internal/client"
	"aibank/bff-onboarding/internal/resolver"
	"aibank/bff-onboarding/internal/schema"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/graphql-go/handler"
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func main() {
	onboardingURL := getEnv("ONBOARDING_URL", "http://onboarding-orchestrator:8084")
	clientURL := getEnv("CLIENT_URL", "http://client-service:8087")
	documentURL := getEnv("DOCUMENT_URL", "http://document-service:8082")
	identityURL := getEnv("IDENTITY_URL", "http://identity-service:8086")

	svc := client.NewServiceClients(onboardingURL, clientURL, documentURL, identityURL)
	res := resolver.New(svc)

	gqlSchema, err := schema.Build(res)
	if err != nil {
		log.Fatalf("bff: failed to build schema: %v", err)
	}

	gqlHandler := handler.New(&handler.Config{
		Schema:   &gqlSchema,
		Pretty:   true,
		GraphiQL: true,
	})

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Handle("/graphql", gqlHandler)

	srv := &http.Server{
		Addr:         ":8094",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("bff-onboarding: listening on :8094")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bff: server error: %v", err)
		}
	}()

	<-quit
	log.Println("bff-onboarding: shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("bff: forced shutdown: %v", err)
	}

	log.Println("bff-onboarding: stopped")
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/bff-admin/internal/client"
	"aibank/bff-admin/internal/resolver"
	"aibank/bff-admin/internal/schema"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/graphql-go/handler"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	tenantURL := getenv("TENANT_URL", "http://tenant-service:8080")
	auditURL := getenv("AUDIT_URL", "http://audit-service:8081")
	riskURL := getenv("RISK_URL", "http://risk-engine:8085")
	clientURL := getenv("CLIENT_URL", "http://client-service:8087")
	uboURL := getenv("UBO_URL", "http://ubo-service:8088")

	clients := client.NewAdminClients(tenantURL, auditURL, riskURL, clientURL, uboURL)
	res := resolver.New(clients)

	gqlSchema, err := schema.Build(res)
	if err != nil {
		log.Fatalf("bff-admin: failed to build GraphQL schema: %v", err)
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

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"bff-admin"}`))
	})

	r.Handle("/graphql", gqlHandler)

	srv := &http.Server{
		Addr:         ":8095",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("bff-admin: listening on :8095")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bff-admin: server error: %v", err)
		}
	}()

	<-stop
	log.Println("bff-admin: shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("bff-admin: graceful shutdown failed: %v", err)
	}
	log.Println("bff-admin: stopped")
}

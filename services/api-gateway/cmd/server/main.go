package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"aibank/api-gateway/internal/middleware"
	"aibank/api-gateway/internal/proxy"
)

func main() {
	// Read optional JWT public key from env
	var publicKeyPEM []byte
	if keyStr := os.Getenv("JWT_PUBLIC_KEY_PEM"); keyStr != "" {
		publicKeyPEM = []byte(keyStr)
	}

	// Rate limiter: 100 tokens max, refill at 10 rps
	rateLimiter := middleware.NewRateLimiter(100, 10)

	// Build reverse proxy router with default routes
	proxyRouter, err := proxy.NewRouter(proxy.DefaultRoutes)
	if err != nil {
		log.Fatalf("failed to build proxy router: %v", err)
	}

	r := chi.NewRouter()

	// Global middleware: request ID + structured access logging
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Health check — no auth required
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// All other routes: tenant extraction → rate limiting → JWT auth → proxy
	r.With(
		middleware.TenantFromSubdomain,
		rateLimiter.Middleware,
		middleware.JWTAuth(publicKeyPEM),
	).HandleFunc("/*", proxyRouter.ServeHTTP)

	srv := &http.Server{
		Addr:         ":8000",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown via signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		log.Printf("api-gateway listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}

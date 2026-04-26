package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/abs-adapter-cft/internal/handler"
)

func main() {
	h := handler.NewHandler()

	srv := &http.Server{
		Addr:         ":9001",
		Handler:      h.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("abs-adapter-cft: listening on :9001")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("abs-adapter-cft: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("abs-adapter-cft: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("abs-adapter-cft: forced shutdown: %v", err)
	}
	log.Println("abs-adapter-cft: stopped")
}

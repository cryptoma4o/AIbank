package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/abs-adapter-rs-bank/internal/handler"
)

func main() {
	h := handler.NewHandler()

	srv := &http.Server{
		Addr:         ":9003",
		Handler:      h.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("abs-adapter-rs-bank: listening on :9003")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("abs-adapter-rs-bank: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("abs-adapter-rs-bank: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("abs-adapter-rs-bank: forced shutdown: %v", err)
	}
	log.Println("abs-adapter-rs-bank: stopped")
}

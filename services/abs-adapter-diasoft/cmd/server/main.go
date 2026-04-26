package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aibank/abs-adapter-diasoft/internal/handler"
)

func main() {
	h := handler.NewHandler()

	srv := &http.Server{
		Addr:         ":9002",
		Handler:      h.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("abs-adapter-diasoft: listening on :9002")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("abs-adapter-diasoft: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("abs-adapter-diasoft: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("abs-adapter-diasoft: forced shutdown: %v", err)
	}
	log.Println("abs-adapter-diasoft: stopped")
}

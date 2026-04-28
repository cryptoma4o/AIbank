// mock-smev — имитатор SMEV3-эндпоинта ФНС ЕГРЮЛ/ЕГРИП.
//
// Используется в pre-integration окружении: ext-egrul LiveProvider ходит сюда
// вместо реальной ФНС, mock возвращает XML той же формы, что MapXMLToLegalEntity
// ожидает на проде. Запускается отдельным процессом/контейнером на :8500.
//
// ENV:
//
//	PORT — порт HTTP (default 8500)
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aibank/mock-smev/internal/handler"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8500"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler.New(logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("mock-smev: HTTP сервер стартует", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("mock-smev: ошибка сервера", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("mock-smev: останавливаемся")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("mock-smev: graceful shutdown failed", "err", err)
	}
	logger.Info("mock-smev: остановлен")
}

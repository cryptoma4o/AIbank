// outbox-cleanup — небольшой CLI, обёртка над outbox.CleanupPublished.
//
// Назначение: запускаться как Kubernetes CronJob (см. infrastructure/helm/charts/
// aibank-service/templates/cleanup-cronjob.yaml) и периодически удалять
// published-строки старше retention из outbox-таблицы конкретного сервиса.
//
// Конфигурация — только через environment variables (12-factor):
//
//	DATABASE_URL           postgres DSN, обязателен.
//	OUTBOX_TABLE           полное имя outbox-таблицы (e.g. "platform.billing_outbox"),
//	                       обязателен.
//	OUTBOX_RETENTION_DAYS  сколько дней держать published-строки. Default: 7.
//	OUTBOX_BATCH_SIZE      размер пачки для итеративного DELETE. Default: 1000.
//
// Коды выхода:
//
//	0 — cleanup отработал успешно (даже если удалили 0 строк — это норма).
//	1 — ошибка конфигурации (нет DATABASE_URL / OUTBOX_TABLE) или БД-сбой.
//
// Tool НЕ требует Kafka, НЕ публикует ничего, НЕ читает unpublished-строки —
// только DELETE по published_at < NOW() - retention. Безопасен к запуску
// параллельно с relay (см. outbox.CleanupPublished doc — SKIP LOCKED).
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/aibank/platform/packages/outbox"
)

const (
	exitOK              = 0
	exitFailure         = 1
	defaultRetentionDay = 7
	defaultBatchSize    = 1000
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("outbox-cleanup failed", "err", err)
		os.Exit(exitFailure)
	}
	os.Exit(exitOK)
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	log.Info("outbox-cleanup запущен",
		"table", cfg.Table,
		"retention_days", cfg.RetentionDays,
		"batch_size", cfg.BatchSize,
	)

	// Graceful shutdown по SIGTERM/SIGINT (k8s посылает SIGTERM при scale-down /
	// pod eviction; CleanupPublished умеет уважать ctx.Done между итерациями).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Короткий ping — лучше упасть сразу, чем висеть в CleanupPublished.
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// topic не используется в cleanup-сценарии, но конструктор требует non-empty.
	// Передаём sentinel-значение; никаких Kafka-публикаций здесь не происходит.
	ob, err := outbox.New(db, cfg.Table, "outbox-cleanup-noop")
	if err != nil {
		return fmt.Errorf("outbox.New: %w", err)
	}

	retention := time.Duration(cfg.RetentionDays) * 24 * time.Hour
	deleted, err := ob.CleanupPublished(ctx, retention, cfg.BatchSize)
	if err != nil {
		// Если контекст отменили — не считаем это ошибкой run'а (будем
		// пытаться снова на следующем CronJob-tick'е).
		if errors.Is(err, context.Canceled) {
			log.Warn("outbox-cleanup прерван по сигналу", "deleted_so_far", deleted)
			return nil
		}
		return fmt.Errorf("cleanup: %w (deleted_so_far=%d)", err, deleted)
	}

	log.Info("outbox-cleanup завершён", "deleted", deleted)
	return nil
}

type config struct {
	DatabaseURL   string
	Table         string
	RetentionDays int
	BatchSize     int
}

func loadConfig() (*config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	table := os.Getenv("OUTBOX_TABLE")
	if table == "" {
		return nil, errors.New("OUTBOX_TABLE is required (e.g. platform.billing_outbox)")
	}

	retentionDays := defaultRetentionDay
	if v := os.Getenv("OUTBOX_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("OUTBOX_RETENTION_DAYS: %w", err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("OUTBOX_RETENTION_DAYS must be > 0, got %d", n)
		}
		retentionDays = n
	}

	batchSize := defaultBatchSize
	if v := os.Getenv("OUTBOX_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("OUTBOX_BATCH_SIZE: %w", err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("OUTBOX_BATCH_SIZE must be > 0, got %d", n)
		}
		batchSize = n
	}

	return &config{
		DatabaseURL:   dsn,
		Table:         table,
		RetentionDays: retentionDays,
		BatchSize:     batchSize,
	}, nil
}

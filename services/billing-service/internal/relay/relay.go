// Package relay — тонкая обёртка над packages/outbox для billing-service.
//
// Решает три задачи, которые не решает generic-пакет:
//   - конфигурация из ENV (KAFKA_BROKERS, OUTBOX_POLL_INTERVAL);
//   - дефолтное имя таблицы и Kafka topic (platform.billing_outbox /
//     platform.billing.events);
//   - функция Build, возвращающая готовые (Outbox, Relay, Closer) или nil
//     если KAFKA_BROKERS не задан (dev-режим).
package relay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aibank/platform/packages/outbox"
)

const (
	// OutboxTable — полное имя таблицы в platform-схеме (миграция 002).
	OutboxTable = "platform.billing_outbox"

	// KafkaTopic — целевой topic per ADR-0010 § 2.
	KafkaTopic = "platform.billing.events"

	defaultPollInterval = 5 * time.Second
)

// Config — минимальный набор параметров для билинг-relay'я.
type Config struct {
	Brokers      []string      // если пусто — relay не запускается (dev-режим)
	PollInterval time.Duration // 0 → DefaultPollInterval (5s)
}

// LoadConfigFromEnv читает KAFKA_BROKERS (CSV) и OUTBOX_POLL_INTERVAL.
// При отсутствии KAFKA_BROKERS возвращается Config с пустым Brokers — Build()
// в этом случае вернёт nil-relay (это не ошибка, это dev-режим).
func LoadConfigFromEnv() Config {
	cfg := Config{}
	if brokers := strings.TrimSpace(os.Getenv("KAFKA_BROKERS")); brokers != "" {
		for _, b := range strings.Split(brokers, ",") {
			if t := strings.TrimSpace(b); t != "" {
				cfg.Brokers = append(cfg.Brokers, t)
			}
		}
	}
	if raw := strings.TrimSpace(os.Getenv("OUTBOX_POLL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			cfg.PollInterval = d
		}
	}
	return cfg
}

// Bundle — собранный relay со всем нужным для запуска и graceful shutdown.
//
// Outbox остаётся nil в dev-режиме (KAFKA_BROKERS пуст) — handler/repo тогда
// тоже не должны пытаться enqueue (см. repository.WithOutbox).
type Bundle struct {
	Outbox    *outbox.Outbox  // nil если dev-режим
	Relay     *outbox.Relay   // nil если dev-режим
	publisher outbox.Publisher
}

// Build конструирует Bundle. Если cfg.Brokers пуст — возвращает Bundle с
// nil-полями (dev-режим: ни Run(), ни Close() не делают ничего).
func Build(db *sql.DB, cfg Config, log *slog.Logger) (*Bundle, error) {
	if db == nil {
		return nil, errors.New("relay: db is required")
	}
	if log == nil {
		log = slog.Default()
	}

	if len(cfg.Brokers) == 0 {
		log.Info("relay: KAFKA_BROKERS не задан — outbox-relay в dev-режиме (no-op)")
		return &Bundle{}, nil
	}

	ob, err := outbox.New(db, OutboxTable, KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("relay: outbox init: %w", err)
	}

	pub := outbox.NewKafkaPublisher(cfg.Brokers, KafkaTopic)

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	r, err := outbox.NewRelay(ob, pub, outbox.RelayOptions{
		PollInterval: pollInterval,
		Logger:       log,
	})
	if err != nil {
		_ = pub.Close()
		return nil, fmt.Errorf("relay: build: %w", err)
	}

	log.Info("relay: outbox-relay сконфигурирован",
		"brokers", cfg.Brokers,
		"topic", KafkaTopic,
		"table", OutboxTable,
		"poll_interval", pollInterval,
	)
	return &Bundle{Outbox: ob, Relay: r, publisher: pub}, nil
}

// Run запускает relay в текущей goroutine (как правило вызывается через `go`
// в main.go). В dev-режиме (Bundle.Relay == nil) сразу возвращает nil.
func (b *Bundle) Run(ctx context.Context) error {
	if b == nil || b.Relay == nil {
		<-ctx.Done()
		return nil
	}
	return b.Relay.Run(ctx)
}

// Close освобождает Kafka-writer. В dev-режиме — noop.
func (b *Bundle) Close() error {
	if b == nil || b.publisher == nil {
		return nil
	}
	return b.publisher.Close()
}

package relay

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/aibank/platform/packages/outbox"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestLoadConfigFromEnv_ParsesBrokersAndInterval — простая проверка парсинга
// двух env-переменных, описанных в README/main.go.
func TestLoadConfigFromEnv_ParsesBrokersAndInterval(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", " kafka-1:9092 , kafka-2:9092,, kafka-3:9092 ")
	t.Setenv("OUTBOX_POLL_INTERVAL", "750ms")

	cfg := LoadConfigFromEnv()

	wantBrokers := []string{"kafka-1:9092", "kafka-2:9092", "kafka-3:9092"}
	if len(cfg.Brokers) != len(wantBrokers) {
		t.Fatalf("brokers len = %d, want %d (%v)", len(cfg.Brokers), len(wantBrokers), cfg.Brokers)
	}
	for i, b := range cfg.Brokers {
		if b != wantBrokers[i] {
			t.Errorf("brokers[%d] = %q, want %q", i, b, wantBrokers[i])
		}
	}
	if cfg.PollInterval != 750*time.Millisecond {
		t.Errorf("poll interval = %v, want 750ms", cfg.PollInterval)
	}
}

// TestBuild_DevModeWhenNoBrokers — без KAFKA_BROKERS Build возвращает Bundle
// с nil-полями, а Run() сразу завершается при ctx.Cancel.
func TestBuild_DevModeWhenNoBrokers(t *testing.T) {
	t.Parallel()
	// Ensure ENV is clean (parallel test, нельзя trust shared state).
	os.Unsetenv("KAFKA_BROKERS")
	os.Unsetenv("OUTBOX_POLL_INTERVAL")

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	bundle, err := Build(db, Config{}, quietLogger())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if bundle.Outbox != nil || bundle.Relay != nil {
		t.Errorf("expected nil Outbox/Relay in dev mode, got %+v", bundle)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bundle.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run dev-mode returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dev-mode Run did not return after ctx cancel")
	}

	if err := bundle.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestBuild_ConfiguresRelay — с непустым brokers Build строит outbox и relay.
// Не запускаем Run (он бы пытался ходить в Kafka на несуществующий брокер).
func TestBuild_ConfiguresRelay(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	bundle, err := Build(db, Config{
		Brokers:      []string{"kafka:9092"},
		PollInterval: 100 * time.Millisecond,
	}, quietLogger())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer bundle.Close()

	if bundle.Outbox == nil {
		t.Fatal("expected non-nil Outbox")
	}
	if bundle.Relay == nil {
		t.Fatal("expected non-nil Relay")
	}
	if bundle.Outbox.Table() != OutboxTable {
		t.Errorf("table = %q, want %q", bundle.Outbox.Table(), OutboxTable)
	}
	if bundle.Outbox.Topic() != KafkaTopic {
		t.Errorf("topic = %q, want %q", bundle.Outbox.Topic(), KafkaTopic)
	}
}

// TestBundle_RunWithStubPublisher — smoke-test полного pipeline: ставим
// в outbox-таблицу одну строку через sqlmock, запускаем relay тиком, проверяем
// что StubPublisher получил сообщение. Это интеграция уровня billing-service +
// packages/outbox, использующая stub publisher вместо реальной Kafka.
func TestBundle_RunWithStubPublisher(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}).
			AddRow(int64(101), "evt-101", []byte(`{"id":"evt-101","tenant_id":"bank-alpha"}`), 0))
	mock.ExpectExec(`UPDATE platform.billing_outbox SET published_at`).
		WithArgs("{101}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ob, err := outbox.New(db, OutboxTable, KafkaTopic)
	if err != nil {
		t.Fatalf("outbox.New: %v", err)
	}
	stub := &outbox.StubPublisher{}
	r, err := outbox.NewRelay(ob, stub, outbox.RelayOptions{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}

	if err := r.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(stub.Messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(stub.Messages))
	}
	if string(stub.Messages[0].Key) != "evt-101" {
		t.Errorf("key = %q, want evt-101", stub.Messages[0].Key)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

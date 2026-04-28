package outbox

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// quietLogger — slog.Logger, отбрасывающий вывод (тесты не должны шуметь).
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestOutbox(t *testing.T) (*Outbox, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	ob, err := New(db, "platform.billing_outbox", "platform.billing.events")
	if err != nil {
		t.Fatalf("outbox new: %v", err)
	}
	return ob, mock, db
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	cases := []struct {
		name        string
		db          *sql.DB
		table       string
		topic       string
		wantErrSubs string
	}{
		{"nil db", nil, "t", "topic", "db is required"},
		{"empty table", db, "", "topic", "table is required"},
		{"empty topic", db, "t", "", "topic is required"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(c.db, c.table, c.topic)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantErrSubs)
			}
		})
	}
}

func TestNew_DefaultsAndOptions(t *testing.T) {
	t.Parallel()
	db, _, _ := sqlmock.New()
	defer db.Close()

	// Дефолты: dlq = table+"_dead_letter", maxAttempts = 5.
	ob, err := New(db, "platform.billing_outbox", "platform.billing.events")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := ob.DeadLetterTable(), "platform.billing_outbox_dead_letter"; got != want {
		t.Errorf("DeadLetterTable() = %q, want %q", got, want)
	}
	if got, want := ob.MaxAttempts(), DefaultMaxAttempts; got != want {
		t.Errorf("MaxAttempts() = %d, want %d", got, want)
	}

	// Override через NewWithOptions.
	ob2, err := NewWithOptions(db, "platform.billing_outbox", "platform.billing.events", Options{
		DeadLetterTable: "custom.poison_box",
		MaxAttempts:     3,
	})
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if ob2.DeadLetterTable() != "custom.poison_box" {
		t.Errorf("DeadLetterTable() override не сработал: %q", ob2.DeadLetterTable())
	}
	if ob2.MaxAttempts() != 3 {
		t.Errorf("MaxAttempts() override не сработал: %d", ob2.MaxAttempts())
	}
}

func TestEnqueueTx_InsertsRow(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO platform.billing_outbox (aggregate_type, aggregate_id, event_type, payload)`)).
		WithArgs("billing_event", "evt-1", "account_opened.llc", []byte(`{"x":1}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := ob.EnqueueTx(context.Background(), tx,
		"billing_event", "evt-1", "account_opened.llc", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEnqueueTx_Validation(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	mock.ExpectBegin()
	tx, _ := db.Begin()
	defer tx.Rollback()

	if err := ob.EnqueueTx(context.Background(), nil,
		"a", "b", "c", []byte("{}")); err == nil {
		t.Errorf("expected error for nil tx")
	}
	if err := ob.EnqueueTx(context.Background(), tx,
		"", "b", "c", []byte("{}")); err == nil {
		t.Errorf("expected error for empty aggregate_type")
	}
	if err := ob.EnqueueTx(context.Background(), tx,
		"a", "b", "c", nil); err == nil {
		t.Errorf("expected error for empty payload")
	}
}

// TestRelay_PicksUpUnpublishedRow — основной happy-path: одна unpublished-строка,
// один tick, Publish вызвался, строка помечена published.
func TestRelay_PicksUpUnpublishedRow(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}).
			AddRow(int64(42), "evt-42", []byte(`{"hello":"world"}`), 0))
	mock.ExpectExec(`UPDATE platform.billing_outbox SET published_at`).
		WithArgs("{42}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	stub := &StubPublisher{}
	relay, err := NewRelay(ob, stub, RelayOptions{Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if err := relay.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if len(stub.Messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(stub.Messages))
	}
	if string(stub.Messages[0].Key) != "evt-42" {
		t.Errorf("key = %q, want evt-42", stub.Messages[0].Key)
	}
	if string(stub.Messages[0].Value) != `{"hello":"world"}` {
		t.Errorf("value = %q, want hello/world JSON", stub.Messages[0].Value)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestRelay_SkipsPublishedRows — если в outbox нет unpublished-строк
// (партиал-индекс по published_at IS NULL отфильтровал всё), Tick
// просто коммитит пустую транзакцию и не вызывает Publish.
func TestRelay_SkipsPublishedRows(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}))
	// UPDATE НЕ должен вызываться, потому что publishedIDs пуст.
	mock.ExpectCommit()

	stub := &StubPublisher{}
	relay, _ := NewRelay(ob, stub, RelayOptions{Logger: quietLogger()})

	if err := relay.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(stub.Messages) != 0 {
		t.Errorf("expected 0 published, got %d", len(stub.Messages))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestRelay_PublishFailure_IncrementsAttempts — Publish провалился, но attempts
// ещё не достиг max — строка остаётся в outbox с обновлённым attempts/last_error.
// На следующем Tick'е попробуем снова.
func TestRelay_PublishFailure_IncrementsAttempts(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}).
			AddRow(int64(7), "evt-7", []byte(`{"v":1}`), 2))
	// UPDATE attempts/last_error для row id=7 (newAttempts=3, max=5 — ещё retry).
	mock.ExpectExec(`UPDATE platform.billing_outbox SET attempts = .+ last_error = .+ last_attempt_at`).
		WithArgs(3, "kafka down", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	failingPub := &StubPublisher{Err: errors.New("kafka down")}
	relay, _ := NewRelay(ob, failingPub, RelayOptions{Logger: quietLogger()})

	if err := relay.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(failingPub.Messages) != 0 {
		t.Errorf("expected 0 successful messages, got %d", len(failingPub.Messages))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestRelay_MovesToDeadLetterAfterMaxAttempts — строка с attempts уже достигшим
// max-1, после очередного провала Publish должна попасть в DLQ (INSERT + DELETE).
func TestRelay_MovesToDeadLetterAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	// MaxAttempts=3 для быстрого теста.
	ob, _ := NewWithOptions(db, "platform.billing_outbox", "platform.billing.events",
		Options{MaxAttempts: 3})

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}).
			AddRow(int64(99), "evt-99", []byte(`{"poison":true}`), 2))
	// newAttempts = 3 == max → INSERT в DLQ, DELETE из outbox.
	mock.ExpectExec(`INSERT INTO platform\.billing_outbox_dead_letter`).
		WithArgs(3, "kafka down", int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id`).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	failingPub := &StubPublisher{Err: errors.New("kafka down")}
	relay, _ := NewRelay(ob, failingPub, RelayOptions{Logger: quietLogger()})

	if err := relay.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestRelay_MixedBatch_PublishedAndFailedAndDLQ — batch из 3 строк:
// одна публикуется, одна провалила Publish (retry), одна провалила и достигла max → DLQ.
func TestRelay_MixedBatch_PublishedAndFailedAndDLQ(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	ob, _ := NewWithOptions(db, "platform.billing_outbox", "platform.billing.events",
		Options{MaxAttempts: 3})

	mock.ExpectBegin()
	// Три строки в batch: id=1 (attempts=0), id=2 (attempts=1), id=3 (attempts=2).
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}).
			AddRow(int64(1), "evt-1", []byte(`{"a":1}`), 0).
			AddRow(int64(2), "evt-2", []byte(`{"a":2}`), 1).
			AddRow(int64(3), "evt-3", []byte(`{"a":3}`), 2))

	// id=1 published, id=2 → retry attempts=2, id=3 → DLQ.
	mock.ExpectExec(`UPDATE platform.billing_outbox SET published_at`).
		WithArgs("{1}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE platform.billing_outbox SET attempts = .+ last_error`).
		WithArgs(2, "kafka partial", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO platform\.billing_outbox_dead_letter`).
		WithArgs(3, "kafka partial", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id`).
		WithArgs(int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	pub := &flakyPublisher{succeedKey: "evt-1", err: errors.New("kafka partial")}
	relay, _ := NewRelay(ob, pub, RelayOptions{Logger: quietLogger()})

	if err := relay.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// flakyPublisher — публикует только если key совпадает с succeedKey.
type flakyPublisher struct {
	succeedKey string
	err        error
	published  []StubMessage
}

func (f *flakyPublisher) Publish(_ context.Context, key, value []byte) error {
	if string(key) == f.succeedKey {
		f.published = append(f.published, StubMessage{Key: append([]byte(nil), key...), Value: append([]byte(nil), value...)})
		return nil
	}
	return f.err
}
func (f *flakyPublisher) Close() error { return nil }

// TestRelay_Run_StopsOnContextCancel — Run() корректно завершается при ctx.Done.
func TestRelay_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ob, mock, db := newTestOutbox(t)
	defer db.Close()

	// Один пустой tick + race с ctx.Cancel допускаем.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, aggregate_id, payload, attempts FROM platform.billing_outbox`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload", "attempts"}))
	mock.ExpectCommit()

	stub := &StubPublisher{}
	relay, _ := NewRelay(ob, stub, RelayOptions{
		PollInterval: 50 * time.Millisecond,
		Logger:       quietLogger(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()

	// Дать relay сделать первый tick, потом отменить.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after ctx cancel")
	}
}

// TestInt64Array — простой sanity-чек для PG bigint[] литерал-формата.
func TestInt64Array(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []int64
		want string
	}{
		{nil, "{}"},
		{[]int64{}, "{}"},
		{[]int64{42}, "{42}"},
		{[]int64{1, 2, 3}, "{1,2,3}"},
	}
	for _, c := range cases {
		got := int64Array(c.in)
		if got != c.want {
			t.Errorf("int64Array(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStubPublisher_CopiesPayload — StubPublisher должен копировать байты,
// чтобы тесты могли реюзать буфер.
func TestStubPublisher_CopiesPayload(t *testing.T) {
	t.Parallel()
	stub := &StubPublisher{}
	buf := []byte(`{"v":1}`)
	if err := stub.Publish(context.Background(), []byte("k"), buf); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Мутация исходного буфера не должна задеть сохранённое сообщение.
	buf[1] = 'X'
	if string(stub.Messages[0].Value) != `{"v":1}` {
		t.Errorf("payload mutated: %s", stub.Messages[0].Value)
	}
}

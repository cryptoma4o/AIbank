package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"aibank/billing-service/internal/domain"
)

// fakeOutbox — реализация OutboxEnqueuer для тестов: запоминает один payload.
type fakeOutbox struct {
	called      bool
	aggregateID string
	eventType   string
	payload     []byte
}

func (f *fakeOutbox) EnqueueTx(
	_ context.Context,
	_ *sql.Tx,
	_, aggregateID, eventType string,
	payload []byte,
) error {
	f.called = true
	f.aggregateID = aggregateID
	f.eventType = eventType
	f.payload = append([]byte(nil), payload...)
	return nil
}

func newRepoWithMock(t *testing.T) (*PostgresBillingEventRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return NewPostgresBillingEventRepository(db), mock, db
}

func sampleEvent(t *testing.T) *domain.BillingEvent {
	t.Helper()
	return &domain.BillingEvent{
		ID:               "evt-001",
		TenantID:         "bank-alpha",
		EventType:        domain.EventTypeAccountOpenedLLC,
		SourceService:    "onboarding-orchestrator",
		SourceEventID:    "src-001",
		Quantity:         1,
		UnitPriceKopecks: 80000,
		Metadata:         []byte(`{"app":"x"}`),
		CreatedAt:        time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC),
	}
}

// TestAppend_DirectInsertWhenNoOutbox — без outbox repo делает один ExecContext
// без BEGIN/COMMIT. Эта ветка нужна юнит-тестам handler'а.
func TestAppend_DirectInsertWhenNoOutbox(t *testing.T) {
	t.Parallel()

	repo, mock, db := newRepoWithMock(t)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO platform.billing_events`)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	stored, inserted, err := repo.Append(context.Background(), sampleEvent(t))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !inserted {
		t.Errorf("expected inserted=true, got false")
	}
	if stored.TotalKopecks != 80000 {
		t.Errorf("total = %d, want 80000", stored.TotalKopecks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestAppend_TransactionalOutbox — с outbox repo открывает транзакцию,
// делает INSERT INTO billing_events, вызывает EnqueueTx, коммитит.
func TestAppend_TransactionalOutbox(t *testing.T) {
	t.Parallel()

	repo, mock, db := newRepoWithMock(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO platform.billing_events`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ob := &fakeOutbox{}
	r := repo.WithOutbox(ob)

	evt := sampleEvent(t)
	stored, inserted, err := r.Append(context.Background(), evt)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !inserted {
		t.Errorf("expected inserted=true")
	}
	if stored.ID != "evt-001" {
		t.Errorf("id = %q, want evt-001", stored.ID)
	}
	if !ob.called {
		t.Fatal("outbox EnqueueTx was not called")
	}
	if ob.aggregateID != "evt-001" {
		t.Errorf("outbox aggregate_id = %q, want evt-001", ob.aggregateID)
	}
	if ob.eventType != domain.EventTypeAccountOpenedLLC {
		t.Errorf("outbox event_type = %q, want %q", ob.eventType, domain.EventTypeAccountOpenedLLC)
	}
	if len(ob.payload) == 0 {
		t.Errorf("outbox payload empty")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestAppend_OutboxNotCalledOnIdempotentReinsert — повторный INSERT с тем же
// PK даёт RowsAffected=0; outbox-row создавать не нужно (событие уже было
// отправлено в Kafka в первый раз).
func TestAppend_OutboxNotCalledOnIdempotentReinsert(t *testing.T) {
	t.Parallel()

	repo, mock, db := newRepoWithMock(t)
	defer db.Close()

	// INSERT с RowsAffected=0 (ON CONFLICT DO NOTHING).
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO platform.billing_events`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	// Повторное чтение из platform.billing_events.
	mock.ExpectQuery(`SELECT id, tenant_id`).
		WithArgs("evt-001").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "event_type", "source_service", "source_event_id",
			"quantity", "unit_price_kopecks", "total_kopecks", "metadata",
			"audit_event_id", "created_at", "billed_at",
		}).AddRow(
			"evt-001", "bank-alpha", domain.EventTypeAccountOpenedLLC,
			"onboarding-orchestrator", "src-001",
			1, int64(80000), int64(80000), []byte(`{"app":"x"}`),
			nil, time.Now(), nil,
		))

	ob := &fakeOutbox{}
	r := repo.WithOutbox(ob)

	stored, inserted, err := r.Append(context.Background(), sampleEvent(t))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if inserted {
		t.Errorf("expected inserted=false on conflict")
	}
	if stored.ID != "evt-001" {
		t.Errorf("id = %q", stored.ID)
	}
	if ob.called {
		t.Error("outbox EnqueueTx must NOT be called on idempotent re-insert")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

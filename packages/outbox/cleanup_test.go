package outbox

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// newCleanupTestOutbox — отдельный helper, потому что cleanup_test и
// outbox_test параллельны и newTestOutbox в outbox_test.go не экспортирован
// между файлами (он экспортирован, просто другой fixture для прозрачности).
func newCleanupTestOutbox(t *testing.T) (*Outbox, sqlmock.Sqlmock, *sql.DB) {
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

// TestCleanupPublished_HappyPath — есть 10 published-строк старше retention,
// batchSize=100 → одна итерация, 10 удалено, выходим.
func TestCleanupPublished_HappyPath(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	// Один DELETE, вернувший 10 строк (< batchSize=100) → loop завершается.
	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id IN \(`).
		WillReturnResult(sqlmock.NewResult(0, 10))

	deleted, err := ob.CleanupPublished(context.Background(), 7*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted != 10 {
		t.Errorf("deleted = %d, want 10", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_NothingToDelete — таблица не содержит published-строк
// старше retention; первый же DELETE возвращает 0 → выход без повторов.
func TestCleanupPublished_NothingToDelete(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id IN \(`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	deleted, err := ob.CleanupPublished(context.Background(), 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_IterativeBatching — 250 строк подходят под retention,
// batchSize=100 → 3 итерации (100 + 100 + 50), всего 250 удалено.
// Критично: на третьей итерации affected < batchSize → loop завершается без
// четвёртого вызова.
func TestCleanupPublished_IterativeBatching(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	deleteRE := `DELETE FROM platform\.billing_outbox WHERE id IN \(`
	// Итерация 1: 100 удалено (== batchSize → продолжаем).
	mock.ExpectExec(deleteRE).
		WillReturnResult(sqlmock.NewResult(0, 100))
	// Итерация 2: 100 удалено (== batchSize → продолжаем).
	mock.ExpectExec(deleteRE).
		WillReturnResult(sqlmock.NewResult(0, 100))
	// Итерация 3: 50 удалено (< batchSize → выход).
	mock.ExpectExec(deleteRE).
		WillReturnResult(sqlmock.NewResult(0, 50))

	deleted, err := ob.CleanupPublished(context.Background(), 7*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted != 250 {
		t.Errorf("deleted = %d, want 250", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_DoesNotTouchUnpublished — sqlmock-уровень: проверяем,
// что сгенерированный SQL содержит published_at IS NOT NULL предикат.
// Это защищает нас от регрессии типа «удалили unpublished-строки».
//
// Дополнительно: подсовываем ExpectExec с явным регэкспом, ожидающим
// published_at IS NOT NULL — если кто-то снимет фильтр, тест упадёт.
func TestCleanupPublished_DoesNotTouchUnpublished(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	// Регулярка специально требует наличия "published_at IS NOT NULL".
	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id IN \(\s*` +
		`SELECT id FROM platform\.billing_outbox\s+` +
		`WHERE published_at IS NOT NULL`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	deleted, err := ob.CleanupPublished(context.Background(), 7*24*time.Hour, 100)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (no published rows old enough)", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_NegativeRetention — защита от ноги выстрела:
// отрицательный retention должен вернуть ошибку, не выполнять DELETE.
func TestCleanupPublished_NegativeRetention(t *testing.T) {
	t.Parallel()

	ob, _, db := newCleanupTestOutbox(t)
	defer db.Close()

	deleted, err := ob.CleanupPublished(context.Background(), -1*time.Second, 100)
	if err == nil {
		t.Fatalf("expected error for negative retention, got nil")
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 on validation error", deleted)
	}
}

// TestCleanupPublished_DefaultBatchSize — batchSize <= 0 → используется
// DefaultCleanupBatchSize (1000). Проверяем, что DELETE содержит LIMIT 1000.
func TestCleanupPublished_DefaultBatchSize(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	// Регулярка с явным LIMIT 1000.
	mock.ExpectExec(regexp.QuoteMeta("LIMIT 1000")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	deleted, err := ob.CleanupPublished(context.Background(), 7*24*time.Hour, 0)
	if err != nil {
		t.Fatalf("CleanupPublished: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_ContextCancellation — если ctx отменён до вызова,
// функция выходит сразу с context-ошибкой, не выполняя DELETE.
//
// (Между итерациями ctx-чек делается тоже — но эту ветку sqlmock'ом
// надёжно не отрепродуцируешь без race; зато early-exit ловит её на старте.)
func TestCleanupPublished_ContextCancellation(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменён до первого Exec

	// Никаких ExpectExec — мы должны выйти раньше, чем ударим в БД.
	_, err := ob.CleanupPublished(ctx, 7*24*time.Hour, 100)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestCleanupPublished_DBError — DELETE возвращает ошибку → функция возвращает
// её, обернув в outbox cleanup: delete:.
func TestCleanupPublished_DBError(t *testing.T) {
	t.Parallel()

	ob, mock, db := newCleanupTestOutbox(t)
	defer db.Close()

	mock.ExpectExec(`DELETE FROM platform\.billing_outbox WHERE id IN \(`).
		WillReturnError(errors.New("connection refused"))

	deleted, err := ob.CleanupPublished(context.Background(), 7*24*time.Hour, 100)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 on DB error", deleted)
	}
}

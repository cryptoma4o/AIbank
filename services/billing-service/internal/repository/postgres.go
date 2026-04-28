package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"aibank/billing-service/internal/domain"
)

// ErrNotFound возвращается, когда событие не найдено.
var ErrNotFound = errors.New("billing event not found")

// defaultListLimit — safety cap для ListByTenant при limit=0.
const defaultListLimit = 1000

// OutboxEnqueuer — узкий интерфейс над outbox.Outbox, чтобы repository не
// тащил весь пакет в зависимости. Append вызывает EnqueueTx внутри той же
// транзакции, что и доменный INSERT — это и есть transactional outbox
// pattern (ADR-0010 § 2).
type OutboxEnqueuer interface {
	EnqueueTx(
		ctx context.Context,
		tx *sql.Tx,
		aggregateType, aggregateID, eventType string,
		payload []byte,
	) error
}

// PostgresBillingEventRepository пишет в platform.billing_events
// (cross-tenant, ADR-0010 § 3 — НЕ tnt_<id>-схема).
//
// Если outbox != nil, Append оборачивает INSERT и outbox-enqueue в одну
// транзакцию. Если nil — старое поведение (просто INSERT). Это позволяет
// существующим unit-тестам работать без Kafka/outbox-таблицы.
type PostgresBillingEventRepository struct {
	db     *sql.DB
	outbox OutboxEnqueuer
}

func NewPostgresBillingEventRepository(db *sql.DB) *PostgresBillingEventRepository {
	return &PostgresBillingEventRepository{db: db}
}

// WithOutbox возвращает копию репозитория с включённым transactional outbox.
// Передайте nil чтобы выключить (dev-режим, тесты).
func (r *PostgresBillingEventRepository) WithOutbox(o OutboxEnqueuer) *PostgresBillingEventRepository {
	cp := *r
	cp.outbox = o
	return &cp
}

// Append идемпотентен по PK (id): на конфликт — возвращает существующую запись
// и inserted=false. Это обязательное поведение per ADR-0010 § 5.
//
// При наличии outbox: INSERT в platform.billing_events и INSERT в
// platform.billing_outbox происходят в одной транзакции. Если был конфликт
// (idempotent re-insert), outbox-запись НЕ создаётся (на повторе мы и так
// уже отправили событие в Kafka в первый раз — иначе у consumer'а будет
// дубликат, который ON CONFLICT DO NOTHING тоже корректно отбросит, но мы
// экономим Kafka-трафик).
func (r *PostgresBillingEventRepository) Append(
	ctx context.Context,
	evt *domain.BillingEvent,
) (*domain.BillingEvent, bool, error) {
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	if evt.Quantity <= 0 {
		evt.Quantity = 1
	}
	evt.TotalKopecks = domain.ComputeTotal(evt.Quantity, evt.UnitPriceKopecks)

	metadata := evt.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}

	var auditEventID sql.NullString
	if evt.AuditEventID != "" {
		auditEventID = sql.NullString{String: evt.AuditEventID, Valid: true}
	}

	// Без outbox — старая, простая ветка (используется в unit-тестах).
	if r.outbox == nil {
		return r.appendDirect(ctx, evt, metadata, auditEventID)
	}

	// С outbox — обе вставки в одной транзакции.
	return r.appendWithOutbox(ctx, evt, metadata, auditEventID)
}

func (r *PostgresBillingEventRepository) appendDirect(
	ctx context.Context,
	evt *domain.BillingEvent,
	metadata []byte,
	auditEventID sql.NullString,
) (*domain.BillingEvent, bool, error) {
	res, err := r.db.ExecContext(ctx, insertEventSQL,
		evt.ID, evt.TenantID, evt.EventType, evt.SourceService, evt.SourceEventID,
		evt.Quantity, evt.UnitPriceKopecks, evt.TotalKopecks, metadata,
		auditEventID, evt.CreatedAt,
	)
	if err != nil {
		return nil, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 0 {
		stored, gerr := r.GetByID(ctx, evt.ID)
		if gerr != nil {
			return nil, false, gerr
		}
		return stored, false, nil
	}
	return evt, true, nil
}

func (r *PostgresBillingEventRepository) appendWithOutbox(
	ctx context.Context,
	evt *domain.BillingEvent,
	metadata []byte,
	auditEventID sql.NullString,
) (*domain.BillingEvent, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }() // noop при успешном Commit

	res, err := tx.ExecContext(ctx, insertEventSQL,
		evt.ID, evt.TenantID, evt.EventType, evt.SourceService, evt.SourceEventID,
		evt.Quantity, evt.UnitPriceKopecks, evt.TotalKopecks, metadata,
		auditEventID, evt.CreatedAt,
	)
	if err != nil {
		return nil, false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, false, err
	}

	if rows == 0 {
		// Идемпотентный re-insert: событие уже было отправлено в Kafka
		// в первый раз, outbox-row создавать не нужно. Просто читаем
		// существующую запись и коммитим (read-only tx безвредна).
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		stored, gerr := r.GetByID(ctx, evt.ID)
		if gerr != nil {
			return nil, false, gerr
		}
		return stored, false, nil
	}

	// Атомарно с INSERT записываем outbox-row. Сериализуем уже сохранённое
	// (с TotalKopecks/CreatedAt) состояние, чтобы downstream consumer
	// получил идентичный объект.
	payload, mErr := json.Marshal(evt)
	if mErr != nil {
		return nil, false, mErr
	}
	if err := r.outbox.EnqueueTx(ctx, tx,
		"billing_event", evt.ID, evt.EventType, payload); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return evt, true, nil
}

const insertEventSQL = `INSERT INTO platform.billing_events
	(id, tenant_id, event_type, source_service, source_event_id,
	 quantity, unit_price_kopecks, total_kopecks, metadata,
	 audit_event_id, created_at, billed_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL)
	ON CONFLICT (id) DO NOTHING`

func (r *PostgresBillingEventRepository) GetByID(
	ctx context.Context,
	id string,
) (*domain.BillingEvent, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, event_type, source_service, source_event_id,
		        quantity, unit_price_kopecks, total_kopecks, metadata,
		        audit_event_id, created_at, billed_at
		 FROM platform.billing_events WHERE id = $1`, id)

	evt, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return evt, nil
}

func (r *PostgresBillingEventRepository) ListByTenant(
	ctx context.Context,
	tenantID string,
	from, to time.Time,
	limit int,
) ([]*domain.BillingEvent, error) {
	if limit <= 0 || limit > defaultListLimit {
		limit = defaultListLimit
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, event_type, source_service, source_event_id,
		        quantity, unit_price_kopecks, total_kopecks, metadata,
		        audit_event_id, created_at, billed_at
		 FROM platform.billing_events
		 WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		 ORDER BY created_at DESC
		 LIMIT $4`,
		tenantID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*domain.BillingEvent, 0, 16)
	for rows.Next() {
		evt, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, evt)
	}
	return out, rows.Err()
}

func (r *PostgresBillingEventRepository) AggregateByTenant(
	ctx context.Context,
	tenantID string,
	from, to time.Time,
) (*domain.Aggregate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT event_type, COUNT(*), COALESCE(SUM(total_kopecks), 0)
		 FROM platform.billing_events
		 WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3
		 GROUP BY event_type`,
		tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agg := &domain.Aggregate{
		TenantID:    tenantID,
		From:        from,
		To:          to,
		ByEventType: make(map[string]int64),
	}
	for rows.Next() {
		var eventType string
		var count int
		var sum int64
		if err := rows.Scan(&eventType, &count, &sum); err != nil {
			return nil, err
		}
		agg.ByEventType[eventType] = sum
		agg.TotalKopecks += sum
		agg.EventCount += count
	}
	return agg, rows.Err()
}

// scanner общий интерфейс для *sql.Row и *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(s scanner) (*domain.BillingEvent, error) {
	var (
		evt          domain.BillingEvent
		auditEventID sql.NullString
		billedAt     sql.NullTime
	)
	err := s.Scan(
		&evt.ID, &evt.TenantID, &evt.EventType, &evt.SourceService, &evt.SourceEventID,
		&evt.Quantity, &evt.UnitPriceKopecks, &evt.TotalKopecks, &evt.Metadata,
		&auditEventID, &evt.CreatedAt, &billedAt,
	)
	if err != nil {
		return nil, err
	}
	if auditEventID.Valid {
		evt.AuditEventID = auditEventID.String
	}
	if billedAt.Valid {
		t := billedAt.Time
		evt.BilledAt = &t
	}
	return &evt, nil
}

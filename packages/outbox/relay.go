package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// DefaultPollInterval — компромисс между latency публикации (5 сек худший случай)
// и нагрузкой на БД (12 SELECT'ов в минуту).
const DefaultPollInterval = 5 * time.Second

// DefaultBatchSize — сколько строк забираем за один SELECT. 100 — безопасно для
// типового объёма billing-events (десятки/сотни в минуту на тенант).
const DefaultBatchSize = 100

// Relay — background loop, который выгребает unpublished-строки из outbox и
// публикует их в Kafka, помечая published_at. Provides DLQ для poison messages.
type Relay struct {
	outbox       *Outbox
	pub          Publisher
	pollInterval time.Duration
	batchSize    int
	log          *slog.Logger
}

// RelayOptions — настройки relay. Все поля опциональны.
type RelayOptions struct {
	PollInterval time.Duration
	BatchSize    int
	Logger       *slog.Logger
}

// NewRelay конструирует relay. outbox и pub обязательны.
func NewRelay(o *Outbox, pub Publisher, opts RelayOptions) (*Relay, error) {
	if o == nil {
		return nil, errors.New("outbox: outbox is required")
	}
	if pub == nil {
		return nil, errors.New("outbox: publisher is required")
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Relay{
		outbox:       o,
		pub:          pub,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		log:          logger,
	}, nil
}

// Run блокирует goroutine до ctx.Done(), периодически вызывая Tick().
// Возвращает nil при штатном завершении (ctx cancelled), либо ошибку
// если SELECT/UPDATE падают подряд (relay ошибки публикации не считает
// фатальными — они продолжаются на следующем poll'е).
func (r *Relay) Run(ctx context.Context) error {
	r.log.Info("outbox relay запущен",
		"table", r.outbox.table,
		"dlq_table", r.outbox.dlqTable,
		"topic", r.outbox.topic,
		"poll_interval", r.pollInterval,
		"max_attempts", r.outbox.maxAttempts,
	)

	t := time.NewTicker(r.pollInterval)
	defer t.Stop()

	// Первый tick сразу, без ожидания pollInterval.
	for {
		if err := r.Tick(ctx); err != nil {
			// Tick возвращает ошибки только БД-уровня. Логируем и продолжаем —
			// на следующем тике повторим.
			r.log.Error("outbox relay tick: ошибка БД", "err", err)
		}
		select {
		case <-ctx.Done():
			r.log.Info("outbox relay остановлен")
			return nil
		case <-t.C:
		}
	}
}

// pendingRow — строка, забранная из outbox в текущем тике.
type pendingRow struct {
	id          int64
	aggregateID string
	payload     []byte
	attempts    int
}

// failedRow — pendingRow, для которого Publish провалился; хранит ошибку и
// уже инкрементированное число попыток.
type failedRow struct {
	row         pendingRow
	newAttempts int
	err         error
}

// Tick — один цикл «забрать batch → опубликовать → пометить published / DLQ».
// Концурентность: SELECT ... FOR UPDATE SKIP LOCKED гарантирует, что несколько
// запущенных одновременно relay-инстансов не возьмут одни и те же строки.
//
// Семантика:
//   - Транзакция держится на всё время батча.
//   - При успехе Publish — UPDATE published_at для id'ов в едином UPDATE.
//   - При неуспехе — UPDATE attempts/last_error для id'ов, у которых attempts < max.
//   - При attempts >= max — INSERT в DLQ + DELETE из outbox (за один SQL-блок per row).
//
// Это at-least-once: дубликаты на consumer-стороне дедупятся идемпотентным
// INSERT (ADR-0010 § 5: ON CONFLICT (id) DO NOTHING).
func (r *Relay) Tick(ctx context.Context) error {
	tx, err := r.outbox.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("outbox relay: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // noop при успешном Commit

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(
		`SELECT id, aggregate_id, payload, attempts FROM %s
		 WHERE published_at IS NULL
		 ORDER BY id
		 LIMIT %d
		 FOR UPDATE SKIP LOCKED`,
		r.outbox.table, r.batchSize))
	if err != nil {
		return fmt.Errorf("outbox relay: select: %w", err)
	}

	batch := make([]pendingRow, 0, r.batchSize)
	for rows.Next() {
		var p pendingRow
		if err := rows.Scan(&p.id, &p.aggregateID, &p.payload, &p.attempts); err != nil {
			rows.Close()
			return fmt.Errorf("outbox relay: scan: %w", err)
		}
		batch = append(batch, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("outbox relay: rows: %w", err)
	}
	rows.Close()

	if len(batch) == 0 {
		return tx.Commit() // ничего не было — быстрый выход
	}

	publishedIDs := make([]int64, 0, len(batch))
	failed := make([]failedRow, 0)
	for _, p := range batch {
		if err := r.pub.Publish(ctx, []byte(p.aggregateID), p.payload); err != nil {
			failed = append(failed, failedRow{
				row:         p,
				newAttempts: p.attempts + 1,
				err:         err,
			})
			continue
		}
		publishedIDs = append(publishedIDs, p.id)
	}

	if len(publishedIDs) > 0 {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET published_at = NOW() WHERE id = ANY($1)`,
			r.outbox.table), int64Array(publishedIDs)); err != nil {
			return fmt.Errorf("outbox relay: mark published: %w", err)
		}
	}

	movedToDLQ := 0
	for _, f := range failed {
		if f.newAttempts >= r.outbox.maxAttempts {
			if err := r.moveToDeadLetterTx(ctx, tx, f); err != nil {
				return fmt.Errorf("outbox relay: move to DLQ: %w", err)
			}
			movedToDLQ++
			r.log.Error("outbox relay: poison message → DLQ",
				"id", f.row.id,
				"aggregate_id", f.row.aggregateID,
				"attempts", f.newAttempts,
				"err", f.err,
			)
			continue
		}
		// Привычная ошибка — записываем attempts/last_error, оставляем для retry.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET attempts = $1, last_error = $2, last_attempt_at = NOW()
			 WHERE id = $3`,
			r.outbox.table),
			f.newAttempts, f.err.Error(), f.row.id); err != nil {
			return fmt.Errorf("outbox relay: update attempts: %w", err)
		}
		r.log.Warn("outbox relay: ошибка публикации, повторим",
			"id", f.row.id,
			"aggregate_id", f.row.aggregateID,
			"attempts", f.newAttempts,
			"max_attempts", r.outbox.maxAttempts,
			"err", f.err,
		)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("outbox relay: commit: %w", err)
	}

	r.log.Debug("outbox relay: tick завершён",
		"selected", len(batch),
		"published", len(publishedIDs),
		"failed", len(failed),
		"dlq", movedToDLQ,
	)
	return nil
}

// moveToDeadLetterTx переносит одну строку из outbox в DLQ-таблицу в указанной
// транзакции. Использует RETURNING и идёт двумя SQL: INSERT с SELECT FROM
// outbox + DELETE FROM outbox.
func (r *Relay) moveToDeadLetterTx(ctx context.Context, tx *sql.Tx, f failedRow) error {
	insertSQL := fmt.Sprintf(
		`INSERT INTO %s (id, aggregate_type, aggregate_id, event_type, payload, created_at, attempts, last_error)
		 SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at, $1, $2
		 FROM %s WHERE id = $3`,
		r.outbox.dlqTable, r.outbox.table)
	if _, err := tx.ExecContext(ctx, insertSQL, f.newAttempts, f.err.Error(), f.row.id); err != nil {
		return fmt.Errorf("insert dlq: %w", err)
	}
	deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, r.outbox.table)
	if _, err := tx.ExecContext(ctx, deleteSQL, f.row.id); err != nil {
		return fmt.Errorf("delete from outbox: %w", err)
	}
	return nil
}

// int64Array — небольшой helper для передачи []int64 как PostgreSQL bigint[].
// Импорт lib/pq.Array привязал бы к конкретному driver'у; здесь же мы строим
// строку формата "{1,2,3}", который PG принимает для bigint[] литералов.
func int64Array(xs []int64) string {
	if len(xs) == 0 {
		return "{}"
	}
	out := make([]byte, 0, 2+len(xs)*8)
	out = append(out, '{')
	for i, x := range xs {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, []byte(fmt.Sprintf("%d", x))...)
	}
	out = append(out, '}')
	return string(out)
}

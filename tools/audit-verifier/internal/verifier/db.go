package verifier

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"time"
)

// DBStore читает события напрямую из PostgreSQL audit-service'а
// (схема `audit`, таблица `events`). Это режим для инцидент-респондера
// с DBA-credentials — он независим от состояния audit-service-pod'а
// и обходит любую логику handler'а (важно для forensic).
//
// Источник: см. services/audit-service/internal/repository/postgres.go.
// Колонки и порядок ОБЯЗАНЫ совпадать.
type DBStore struct {
	DB *sql.DB
	// ChunkSize — размер пачки в одном COPY/SELECT-итерации. По умолчанию 1000.
	ChunkSize int
}

// NewDBStore конструирует store. db должна быть открыта вызывающим
// и закрыта после работы.
func NewDBStore(db *sql.DB) *DBStore {
	return &DBStore{DB: db, ChunkSize: 1000}
}

// ListEvents — стримит события ASC по created_at (хронологически).
// Использует database/sql Rows.Next для построчного чтения, чтобы
// не материализовать миллионы строк в памяти.
func (s *DBStore) ListEvents(ctx context.Context, tenantID string, from, to *time.Time) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		// from/to передаём как nullable-параметры; в SQL — IS NULL OR ...
		// чтобы избежать построения динамического SQL (anti SQL injection).
		var fromArg, toArg any
		if from != nil {
			fromArg = *from
		}
		if to != nil {
			toArg = *to
		}

		rows, err := s.DB.QueryContext(ctx, `
			SELECT id, tenant_id, entity_type, entity_id, event_type,
			       actor_id, actor_type, payload, previous_hash, hash, created_at,
			       signature, signature_algorithm, signer_key_id
			FROM audit.events
			WHERE tenant_id = $1
			  AND ($2::timestamptz IS NULL OR created_at >= $2)
			  AND ($3::timestamptz IS NULL OR created_at <= $3)
			ORDER BY created_at ASC, id ASC`,
			tenantID, fromArg, toArg)
		if err != nil {
			yield(Event{}, fmt.Errorf("query events: %w", err))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var e Event
			var sig sql.RawBytes
			var sigAlg, signerID sql.NullString
			if err := rows.Scan(
				&e.ID, &e.TenantID, &e.EntityType, &e.EntityID, &e.EventType,
				&e.ActorID, &e.ActorType, &e.Payload, &e.PreviousHash, &e.Hash, &e.CreatedAt,
				&sig, &sigAlg, &signerID,
			); err != nil {
				yield(Event{}, fmt.Errorf("scan event: %w", err))
				return
			}
			if len(sig) > 0 {
				e.Signature = append([]byte(nil), sig...)
			}
			if sigAlg.Valid {
				e.SignatureAlgorithm = sigAlg.String
			}
			if signerID.Valid {
				e.SignerKeyID = signerID.String
			}
			if !yield(e, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(Event{}, fmt.Errorf("rows iteration: %w", err))
		}
	}
}

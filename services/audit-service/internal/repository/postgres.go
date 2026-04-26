package repository

import (
	"context"
	"database/sql"

	"github.com/aibank/platform/services/audit-service/internal/domain"
)

type PostgresAuditRepository struct {
	db *sql.DB
}

func NewPostgresAuditRepository(db *sql.DB) *PostgresAuditRepository {
	return &PostgresAuditRepository{db: db}
}

func (r *PostgresAuditRepository) Append(ctx context.Context, e *domain.AuditEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit.events
		 (id, tenant_id, entity_type, entity_id, event_type, actor_id, actor_type,
		  payload, previous_hash, hash, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.ID, e.TenantID, e.EntityType, e.EntityID, e.EventType,
		e.ActorID, e.ActorType, e.Payload, e.PreviousHash, e.Hash, e.CreatedAt)
	return err
}

func (r *PostgresAuditRepository) LatestHash(ctx context.Context, tenantID string) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx,
		`SELECT hash FROM audit.events WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 1`,
		tenantID).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil // genesis event
	}
	return hash, err
}

func (r *PostgresAuditRepository) List(ctx context.Context, tenantID, entityType, entityID string, limit int) ([]*domain.AuditEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, entity_type, entity_id, event_type, actor_id, actor_type,
		        payload, previous_hash, hash, created_at
		 FROM audit.events
		 WHERE tenant_id=$1
		   AND ($2='' OR entity_type=$2)
		   AND ($3='' OR entity_id=$3)
		 ORDER BY created_at DESC LIMIT $4`,
		tenantID, entityType, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.EntityType, &e.EntityID, &e.EventType,
			&e.ActorID, &e.ActorType, &e.Payload, &e.PreviousHash, &e.Hash, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

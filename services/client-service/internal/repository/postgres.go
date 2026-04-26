package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"aibank/client-service/internal/domain"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type ClientRepository struct {
	db *sql.DB
}

func NewClientRepository(db *sql.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) Create(ctx context.Context, c *domain.Client) error {
	c.ID = "cli_" + uuid.New().String()
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	c.Status = domain.ClientStatusActive
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO clients (id, tenant_id, inn, ogrn, full_name, type, status, created_at, updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID, c.TenantID, c.INN, c.OGRN, c.FullName, c.Type, c.Status, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *ClientRepository) GetByID(ctx context.Context, tenantID, clientID string) (*domain.Client, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, inn, ogrn, full_name, type, status, created_at, updated_at
         FROM clients WHERE id=$1 AND tenant_id=$2`, clientID, tenantID)
	var c domain.Client
	if err := row.Scan(&c.ID, &c.TenantID, &c.INN, &c.OGRN, &c.FullName, &c.Type, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("client repo: get by id: %w", err)
	}
	return &c, nil
}

func (r *ClientRepository) GetByINN(ctx context.Context, tenantID, inn string) (*domain.Client, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, inn, ogrn, full_name, type, status, created_at, updated_at
         FROM clients WHERE inn=$1 AND tenant_id=$2 LIMIT 1`, inn, tenantID)
	var c domain.Client
	if err := row.Scan(&c.ID, &c.TenantID, &c.INN, &c.OGRN, &c.FullName, &c.Type, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("client repo: get by inn: %w", err)
	}
	return &c, nil
}

func (r *ClientRepository) AppendEvent(ctx context.Context, e *domain.ClientEvent) error {
	e.ID = "cev_" + uuid.New().String()
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	payload := e.Payload
	if payload == nil {
		payload, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO client_events (id, client_id, tenant_id, category, event_type, resource_id, payload, occurred_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID, e.ClientID, e.TenantID, e.Category, e.EventType, e.ResourceID, payload, e.OccurredAt,
	)
	return err
}

func (r *ClientRepository) ListEvents(ctx context.Context, tenantID, clientID string, limit int) ([]domain.ClientEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, client_id, tenant_id, category, event_type, resource_id, payload, occurred_at
         FROM client_events WHERE client_id=$1 AND tenant_id=$2
         ORDER BY occurred_at DESC LIMIT $3`, clientID, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("client repo: list events: %w", err)
	}
	defer rows.Close()
	var events []domain.ClientEvent
	for rows.Next() {
		var e domain.ClientEvent
		if err := rows.Scan(&e.ID, &e.ClientID, &e.TenantID, &e.Category, &e.EventType, &e.ResourceID, &e.Payload, &e.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

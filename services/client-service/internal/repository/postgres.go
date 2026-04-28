// Package repository содержит PostgreSQL-реализации репозиториев client-service.
//
// Изоляция тенантов — schema-per-tenant (ADR-0002). На каждый запрос
// устанавливается LOCAL search_path в схему tnt_<id>, после чего DML/DQL
// идут без префикса схемы. Имя схемы валидируется regex'ом, идентичным
// tenant-service'овскому, для защиты от SQL-injection через идентификатор.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"aibank/client-service/internal/domain"
)

// ErrNotFound возвращается, когда сущность отсутствует в схеме тенанта.
var ErrNotFound = errors.New("not found")

// validTenantID — whitelist идентификатора схемы (ADR-0002, раздел Безопасность).
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q", tenantID)
	}
	return "tnt_" + tenantID, nil
}

// withTenantTx открывает транзакцию, выставляет LOCAL search_path в схему
// тенанта и передаёт управление функции fn. Использование LOCAL гарантирует,
// что соединение возвращается в пул в чистом состоянии.
func withTenantTx(ctx context.Context, db *sql.DB, tenantID string, fn func(*sql.Tx) error) error {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Identifier whitelisted regex'ом → безопасно интерполировать.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`SET LOCAL search_path TO %q, public`, schema)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// PostgresClientRepository реализует domain.ClientRepository.
type PostgresClientRepository struct {
	db *sql.DB
}

// NewPostgresClientRepository — конструктор.
func NewPostgresClientRepository(db *sql.DB) *PostgresClientRepository {
	return &PostgresClientRepository{db: db}
}

// Create вставляет новую карточку клиента в схему тенанта.
func (r *PostgresClientRepository) Create(ctx context.Context, c *domain.Client) error {
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	return withTenantTx(ctx, r.db, c.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO clients
			   (id, applicant_id, legal_entity_id, status, risk_category, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			c.ID, c.ApplicantID, c.LegalEntityID, c.Status, c.RiskCategory, c.CreatedAt, c.UpdatedAt)
		return err
	})
}

// GetByID возвращает клиента по id в рамках тенанта.
func (r *PostgresClientRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Client, error) {
	var c domain.Client
	c.TenantID = tenantID
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT id, applicant_id, legal_entity_id, status, risk_category, created_at, updated_at
			 FROM clients WHERE id = $1`, id)
		err := row.Scan(&c.ID, &c.ApplicantID, &c.LegalEntityID,
			&c.Status, &c.RiskCategory, &c.CreatedAt, &c.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListByTenant выдаёт страницу клиентов одного тенанта, отсортированных по
// убыванию created_at.
func (r *PostgresClientRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*domain.Client, error) {
	clients := make([]*domain.Client, 0)
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, applicant_id, legal_entity_id, status, risk_category, created_at, updated_at
			 FROM clients ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c := &domain.Client{TenantID: tenantID}
			if err := rows.Scan(&c.ID, &c.ApplicantID, &c.LegalEntityID,
				&c.Status, &c.RiskCategory, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
			clients = append(clients, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// UpdateStatus меняет только status и updated_at.
func (r *PostgresClientRepository) UpdateStatus(ctx context.Context, tenantID, id string, status domain.ClientStatus) error {
	return withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE clients SET status = $2, updated_at = $3 WHERE id = $1`,
			id, status, time.Now().UTC())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// PostgresClientHistoryRepository реализует domain.ClientHistoryRepository.
//
// UPDATE и DELETE запрещены на уровне триггера в схеме тенанта (см.
// migrations/tenant/001_create_clients.sql) — Append() это единственный
// разрешённый write-путь.
type PostgresClientHistoryRepository struct {
	db *sql.DB
}

// NewPostgresClientHistoryRepository — конструктор.
func NewPostgresClientHistoryRepository(db *sql.DB) *PostgresClientHistoryRepository {
	return &PostgresClientHistoryRepository{db: db}
}

// Append добавляет событие в client_history. tenantID берём из вызывающего
// контекста (handler), т.к. сама запись в схеме тенанта не содержит tenant_id —
// изоляция обеспечивается схемой.
func (r *PostgresClientHistoryRepository) Append(ctx context.Context, h *domain.ClientHistory, tenantID string) error {
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now().UTC()
	}
	return withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO client_history
			   (id, client_id, event_type, summary, source, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			h.ID, h.ClientID, h.EventType, h.Summary, h.Source, h.CreatedAt)
		return err
	})
}

// ListByClient возвращает историю клиента по убыванию created_at.
func (r *PostgresClientHistoryRepository) ListByClient(ctx context.Context, tenantID, clientID string, limit, offset int) ([]*domain.ClientHistory, error) {
	items := make([]*domain.ClientHistory, 0)
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, client_id, event_type, summary, source, created_at
			 FROM client_history WHERE client_id = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			clientID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			h := &domain.ClientHistory{}
			if err := rows.Scan(&h.ID, &h.ClientID, &h.EventType, &h.Summary, &h.Source, &h.CreatedAt); err != nil {
				return err
			}
			items = append(items, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

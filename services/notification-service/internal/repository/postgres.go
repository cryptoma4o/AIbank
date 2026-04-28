// Package repository — Postgres-репозиторий для notification-service.
//
// Tenant-isolation реализуется через search_path: при подключении к БД
// service устанавливает search_path = "tnt_<tenant_id>",public — все
// запросы автоматически идут в схему тенанта.
//
// Регекс tenant_id (`^[a-z][a-z0-9_]{1,31}$`) синхронизирован с
// tenant-service/internal/repository/postgres.go.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aibank/platform/services/notification-service/internal/domain"
)

// validTenantID — whitelist; защищает SetSearchPath от SQL-injection.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// SetSearchPath устанавливает schema-scope для conn.  Вызывается перед
// группой запросов одного тенанта.
func SetSearchPath(ctx context.Context, db *sql.DB, tenantID string) error {
	if !validTenantID.MatchString(tenantID) {
		return fmt.Errorf("repository: invalid tenant id %q", tenantID)
	}
	schema := "tnt_" + tenantID
	_, err := db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %q, public", schema))
	return err
}

// PostgresNotificationRepository — реализация NotificationRepository.
type PostgresNotificationRepository struct {
	db *sql.DB
}

func NewPostgresNotificationRepository(db *sql.DB) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{db: db}
}

// Create — INSERT в notifications.  Vars сериализуются в JSONB.
func (r *PostgresNotificationRepository) Create(ctx context.Context, n *domain.Notification) error {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	if n.Status == "" {
		n.Status = domain.StatusQueued
	}
	vars, err := json.Marshal(n.Vars)
	if err != nil {
		return fmt.Errorf("marshal vars: %w", err)
	}
	if err := SetSearchPath(ctx, r.db, n.TenantID); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO notifications
		   (id, tenant_id, recipient_type, recipient, template_id, vars,
		    status, attempt_count, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		n.ID, n.TenantID, n.RecipientType, n.Recipient, n.TemplateID, vars,
		n.Status, n.AttemptCount, n.CreatedAt)
	return err
}

// MarkSent — успешная отправка.
func (r *PostgresNotificationRepository) MarkSent(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications
		    SET status='sent', sent_at=NOW(), attempt_count = attempt_count + 1
		  WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MarkFailed — неудачная отправка с причиной.
func (r *PostgresNotificationRepository) MarkFailed(ctx context.Context, id string, errMsg string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications
		    SET status='failed', error=$2, attempt_count = attempt_count + 1
		  WHERE id=$1`, id, errMsg)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListByTenant — выборка с лимитом по убыванию created_at.
func (r *PostgresNotificationRepository) ListByTenant(ctx context.Context, tenantID string, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	if err := SetSearchPath(ctx, r.db, tenantID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, recipient_type, recipient, template_id, vars,
		        status, attempt_count, COALESCE(error,''), sent_at, created_at
		   FROM notifications
		  WHERE tenant_id = $1
		  ORDER BY created_at DESC
		  LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Notification, 0, limit)
	for rows.Next() {
		var n domain.Notification
		var vars []byte
		var sentAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.TenantID, &n.RecipientType, &n.Recipient,
			&n.TemplateID, &vars, &n.Status, &n.AttemptCount, &n.Error,
			&sentAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		if len(vars) > 0 {
			_ = json.Unmarshal(vars, &n.Vars)
		}
		if sentAt.Valid {
			t := sentAt.Time
			n.SentAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetByID — единичная выборка.
func (r *PostgresNotificationRepository) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, recipient_type, recipient, template_id, vars,
		        status, attempt_count, COALESCE(error,''), sent_at, created_at
		   FROM notifications WHERE id=$1`, id)
	var n domain.Notification
	var vars []byte
	var sentAt sql.NullTime
	err := row.Scan(&n.ID, &n.TenantID, &n.RecipientType, &n.Recipient,
		&n.TemplateID, &vars, &n.Status, &n.AttemptCount, &n.Error,
		&sentAt, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(vars) > 0 {
		_ = json.Unmarshal(vars, &n.Vars)
	}
	if sentAt.Valid {
		t := sentAt.Time
		n.SentAt = &t
	}
	return &n, nil
}

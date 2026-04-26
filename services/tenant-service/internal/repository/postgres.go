package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/aibank/platform/services/tenant-service/internal/domain"
)

type PostgresTenantRepository struct {
	db *sql.DB
}

func NewPostgresTenantRepository(db *sql.DB) *PostgresTenantRepository {
	return &PostgresTenantRepository{db: db}
}

func (r *PostgresTenantRepository) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	// NOTE: queries run on the platform schema, not tenant schema
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, bik, inn, status, deployment_mode, created_at, updated_at
		 FROM platform.tenants WHERE id = $1`, id)

	var t domain.Tenant
	err := row.Scan(&t.ID, &t.Name, &t.BIK, &t.INN, &t.Status, &t.DeploymentMode, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant %s not found", id)
	}
	return &t, err
}

func (r *PostgresTenantRepository) List(ctx context.Context) ([]*domain.Tenant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, bik, inn, status, deployment_mode, created_at, updated_at
		 FROM platform.tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.BIK, &t.INN, &t.Status, &t.DeploymentMode, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, &t)
	}
	return tenants, rows.Err()
}

func (r *PostgresTenantRepository) Create(ctx context.Context, t *domain.Tenant) error {
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = t.CreatedAt
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO platform.tenants (id, name, bik, inn, status, deployment_mode, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		t.ID, t.Name, t.BIK, t.INN, t.Status, t.DeploymentMode, t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *PostgresTenantRepository) Update(ctx context.Context, t *domain.Tenant) error {
	t.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE platform.tenants SET name=$2, status=$3, deployment_mode=$4, updated_at=$5 WHERE id=$1`,
		t.ID, t.Name, t.Status, t.DeploymentMode, t.UpdatedAt)
	return err
}

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"aibank/onboarding-orchestrator/internal/domain"
)

// PostgresActivityRepository реализует ApplicationActivityRepository.
type PostgresActivityRepository struct {
	db *sql.DB
}

func NewPostgresActivityRepository(db *sql.DB) *PostgresActivityRepository {
	return &PostgresActivityRepository{db: db}
}

func (r *PostgresActivityRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	stmt := fmt.Sprintf("SET LOCAL search_path TO %s, public", pq.QuoteIdentifier(schema))
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("set search_path: %w", err)
	}
	return tx, nil
}

func (r *PostgresActivityRepository) Upsert(ctx context.Context, a *domain.ApplicationActivity) error {
	tx, err := r.withTenantTx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	suppliersJSON, _ := json.Marshal(orEmpty(a.TopSuppliers))
	buyersJSON, _ := json.Marshal(orEmpty(a.TopBuyers))
	opModelJSON, _ := jsonMarshal(a.OperationalModel)
	fundsJSON, _ := json.Marshal(a.FundsSource)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO application_activities (
			id, tenant_id, application_id,
			business_description, business_category,
			top_suppliers, top_buyers,
			operational_model, funds_source,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (application_id) DO UPDATE SET
			business_description = EXCLUDED.business_description,
			business_category    = EXCLUDED.business_category,
			top_suppliers        = EXCLUDED.top_suppliers,
			top_buyers           = EXCLUDED.top_buyers,
			operational_model    = EXCLUDED.operational_model,
			funds_source         = EXCLUDED.funds_source,
			updated_at           = EXCLUDED.updated_at`,
		a.ID, a.TenantID, a.ApplicationID,
		a.BusinessDescription, a.BusinessCategory,
		suppliersJSON, buyersJSON,
		opModelJSON, fundsJSON,
		a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert activity: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresActivityRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.ApplicationActivity, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id,
		       business_description, business_category,
		       top_suppliers, top_buyers,
		       operational_model, funds_source,
		       created_at, updated_at
		FROM application_activities
		WHERE application_id = $1`, applicationID)

	var (
		a              domain.ApplicationActivity
		suppliersJSON  []byte
		buyersJSON     []byte
		opModelJSON    []byte
		fundsJSON      []byte
	)
	if err := row.Scan(
		&a.ID, &a.TenantID, &a.ApplicationID,
		&a.BusinessDescription, &a.BusinessCategory,
		&suppliersJSON, &buyersJSON,
		&opModelJSON, &fundsJSON,
		&a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if len(suppliersJSON) > 0 {
		if err := json.Unmarshal(suppliersJSON, &a.TopSuppliers); err != nil {
			return nil, fmt.Errorf("decode top_suppliers: %w", err)
		}
	}
	if len(buyersJSON) > 0 {
		if err := json.Unmarshal(buyersJSON, &a.TopBuyers); err != nil {
			return nil, fmt.Errorf("decode top_buyers: %w", err)
		}
	}
	if err := jsonUnmarshalNullable(opModelJSON, &a.OperationalModel); err != nil {
		return nil, err
	}
	if len(fundsJSON) > 0 {
		if err := json.Unmarshal(fundsJSON, &a.FundsSource); err != nil {
			return nil, fmt.Errorf("decode funds_source: %w", err)
		}
	}
	return &a, nil
}

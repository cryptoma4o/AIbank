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

type PostgresAccountRepository struct {
	db *sql.DB
}

func NewPostgresAccountRepository(db *sql.DB) *PostgresAccountRepository {
	return &PostgresAccountRepository{db: db}
}

func (r *PostgresAccountRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresAccountRepository) Upsert(ctx context.Context, a *domain.Account) error {
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

	agreementsJSON, _ := json.Marshal(a.Agreements)

	var openedAt sql.NullTime
	if a.OpenedAt != nil {
		openedAt = sql.NullTime{Time: *a.OpenedAt, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (
			id, tenant_id, application_id, legal_entity_id,
			account_number, bik, bank_name, currency, account_type,
			correspondent_account, tariff_plan,
			agreements, monitoring_profile_id, abs_reference,
			opened_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,
			NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), $8, $9,
			NULLIF($10,''), NULLIF($11,''),
			$12, NULLIF($13,''), NULLIF($14,''),
			$15, $16, $17
		)
		ON CONFLICT (application_id) DO UPDATE SET
			legal_entity_id          = EXCLUDED.legal_entity_id,
			account_number           = EXCLUDED.account_number,
			bik                      = EXCLUDED.bik,
			bank_name                = EXCLUDED.bank_name,
			currency                 = EXCLUDED.currency,
			account_type             = EXCLUDED.account_type,
			correspondent_account    = EXCLUDED.correspondent_account,
			tariff_plan              = EXCLUDED.tariff_plan,
			agreements               = EXCLUDED.agreements,
			monitoring_profile_id    = EXCLUDED.monitoring_profile_id,
			abs_reference            = EXCLUDED.abs_reference,
			opened_at                = EXCLUDED.opened_at,
			updated_at               = EXCLUDED.updated_at`,
		a.ID, a.TenantID, a.ApplicationID, a.LegalEntityID,
		a.AccountNumber, a.BIK, a.BankName, a.Currency, a.AccountType,
		a.CorrespondentAccount, a.TariffPlan,
		agreementsJSON, a.MonitoringProfileID, a.ABSReference,
		openedAt, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert account: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresAccountRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.Account, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id, legal_entity_id,
		       COALESCE(account_number,''), COALESCE(bik,''), COALESCE(bank_name,''),
		       currency, account_type,
		       COALESCE(correspondent_account,''), COALESCE(tariff_plan,''),
		       agreements, COALESCE(monitoring_profile_id,''), COALESCE(abs_reference,''),
		       opened_at, created_at, updated_at
		FROM accounts
		WHERE application_id = $1`, applicationID)

	var (
		a              domain.Account
		agreementsJSON []byte
		openedAt       sql.NullTime
	)
	if err := row.Scan(
		&a.ID, &a.TenantID, &a.ApplicationID, &a.LegalEntityID,
		&a.AccountNumber, &a.BIK, &a.BankName,
		&a.Currency, &a.AccountType,
		&a.CorrespondentAccount, &a.TariffPlan,
		&agreementsJSON, &a.MonitoringProfileID, &a.ABSReference,
		&openedAt, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan account: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if openedAt.Valid {
		t := openedAt.Time
		a.OpenedAt = &t
	}
	if len(agreementsJSON) > 0 {
		if err := json.Unmarshal(agreementsJSON, &a.Agreements); err != nil {
			return nil, fmt.Errorf("decode agreements: %w", err)
		}
	}
	return &a, nil
}

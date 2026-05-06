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

// PostgresProfileRepository — реализация LegalEntityProfileRepository.
// Использует общий withTenantTx для search_path-изоляции тенантов.
type PostgresProfileRepository struct {
	db *sql.DB
}

func NewPostgresProfileRepository(db *sql.DB) *PostgresProfileRepository {
	return &PostgresProfileRepository{db: db}
}

func (r *PostgresProfileRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresProfileRepository) Upsert(ctx context.Context, p *domain.LegalEntityProfile) error {
	tx, err := r.withTenantTx(ctx, p.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	authorizedAmount, authorizedCurrency := splitMoney(p.AuthorizedCapital)
	revenueAmount, revenueCurrency := splitMoney(p.RevenueLastYear)
	legalJSON, _ := jsonMarshal(p.LegalAddress)
	actualJSON, _ := jsonMarshal(p.ActualAddress)
	postalJSON, _ := jsonMarshal(p.PostalAddress)
	contactsJSON, _ := jsonMarshal(p.Contacts)
	licensesJSON, _ := jsonMarshal(orEmpty(p.Licenses))
	sroJSON, _ := jsonMarshal(orEmpty(p.SROMembership))

	_, err = tx.ExecContext(ctx, `
		INSERT INTO legal_entity_profiles (
			id, tenant_id, application_id, legal_entity_id,
			opf_code, registration_authority,
			authorized_capital_amount, authorized_capital_currency,
			legal_address_struct, actual_address_struct, actual_same_as_legal,
			postal_address_struct, postal_same_as_legal,
			okved_main_v2, okved_additional_v2,
			licenses, sro_membership, contacts,
			employees_count, revenue_last_year_amount, revenue_last_year_currency,
			tax_regime, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,
			NULLIF($5,''), NULLIF($6,''),
			$7, NULLIF($8,''),
			$9, $10, $11,
			$12, $13,
			NULLIF($14,''), $15,
			$16, $17, $18,
			$19, $20, NULLIF($21,''),
			NULLIF($22,''), $23, $24
		)
		ON CONFLICT (application_id) DO UPDATE SET
			opf_code                    = EXCLUDED.opf_code,
			registration_authority      = EXCLUDED.registration_authority,
			authorized_capital_amount   = EXCLUDED.authorized_capital_amount,
			authorized_capital_currency = EXCLUDED.authorized_capital_currency,
			legal_address_struct        = EXCLUDED.legal_address_struct,
			actual_address_struct       = EXCLUDED.actual_address_struct,
			actual_same_as_legal        = EXCLUDED.actual_same_as_legal,
			postal_address_struct       = EXCLUDED.postal_address_struct,
			postal_same_as_legal        = EXCLUDED.postal_same_as_legal,
			okved_main_v2               = EXCLUDED.okved_main_v2,
			okved_additional_v2         = EXCLUDED.okved_additional_v2,
			licenses                    = EXCLUDED.licenses,
			sro_membership              = EXCLUDED.sro_membership,
			contacts                    = EXCLUDED.contacts,
			employees_count             = EXCLUDED.employees_count,
			revenue_last_year_amount    = EXCLUDED.revenue_last_year_amount,
			revenue_last_year_currency  = EXCLUDED.revenue_last_year_currency,
			tax_regime                  = EXCLUDED.tax_regime,
			updated_at                  = EXCLUDED.updated_at`,
		p.ID, p.TenantID, p.ApplicationID, p.LegalEntityID,
		p.OPFCode, p.RegistrationAuthority,
		nullableFloat(authorizedAmount), authorizedCurrency,
		legalJSON, actualJSON, p.ActualSameAsLegal,
		postalJSON, p.PostalSameAsLegal,
		p.OKVEDMain, pq.Array(p.OKVEDAdditional),
		licensesJSON, sroJSON, contactsJSON,
		nullableInt(p.EmployeesCount), nullableFloat(revenueAmount), revenueCurrency,
		p.TaxRegime, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresProfileRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.LegalEntityProfile, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id, legal_entity_id,
		       COALESCE(opf_code,''), COALESCE(registration_authority,''),
		       authorized_capital_amount, COALESCE(authorized_capital_currency,''),
		       legal_address_struct, actual_address_struct, actual_same_as_legal,
		       postal_address_struct, postal_same_as_legal,
		       COALESCE(okved_main_v2,''), okved_additional_v2,
		       licenses, sro_membership, contacts,
		       employees_count, revenue_last_year_amount, COALESCE(revenue_last_year_currency,''),
		       COALESCE(tax_regime,''), created_at, updated_at
		FROM legal_entity_profiles
		WHERE application_id = $1`, applicationID)

	var (
		p                domain.LegalEntityProfile
		authAmount       sql.NullFloat64
		authCurrency     string
		legalJSON        []byte
		actualJSON       []byte
		postalJSON       []byte
		licensesJSON     []byte
		sroJSON          []byte
		contactsJSON     []byte
		okvedAdd         pq.StringArray
		employees        sql.NullInt64
		revenueAmount    sql.NullFloat64
		revenueCurrency  string
	)
	if err := row.Scan(
		&p.ID, &p.TenantID, &p.ApplicationID, &p.LegalEntityID,
		&p.OPFCode, &p.RegistrationAuthority,
		&authAmount, &authCurrency,
		&legalJSON, &actualJSON, &p.ActualSameAsLegal,
		&postalJSON, &p.PostalSameAsLegal,
		&p.OKVEDMain, &okvedAdd,
		&licensesJSON, &sroJSON, &contactsJSON,
		&employees, &revenueAmount, &revenueCurrency,
		&p.TaxRegime, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if authAmount.Valid {
		p.AuthorizedCapital = &domain.MoneyAmount{Amount: authAmount.Float64, Currency: authCurrency}
	}
	if revenueAmount.Valid {
		p.RevenueLastYear = &domain.MoneyAmount{Amount: revenueAmount.Float64, Currency: revenueCurrency}
	}
	if employees.Valid {
		v := int(employees.Int64)
		p.EmployeesCount = &v
	}
	p.OKVEDAdditional = []string(okvedAdd)
	if err := jsonUnmarshalNullable(legalJSON, &p.LegalAddress); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(actualJSON, &p.ActualAddress); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(postalJSON, &p.PostalAddress); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(contactsJSON, &p.Contacts); err != nil {
		return nil, err
	}
	if len(licensesJSON) > 0 {
		if err := json.Unmarshal(licensesJSON, &p.Licenses); err != nil {
			return nil, fmt.Errorf("decode licenses: %w", err)
		}
	}
	if len(sroJSON) > 0 {
		if err := json.Unmarshal(sroJSON, &p.SROMembership); err != nil {
			return nil, fmt.Errorf("decode sro: %w", err)
		}
	}
	return &p, nil
}

// ── helpers ─────────────────────────────────────────────────────────

func splitMoney(m *domain.MoneyAmount) (float64, string) {
	if m == nil {
		return 0, ""
	}
	return m.Amount, m.Currency
}

func nullableFloat(v float64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullableInt(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func jsonUnmarshalNullable[T any](raw []byte, dst **T) error {
	if len(raw) == 0 {
		return nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	*dst = &v
	return nil
}

// orEmpty гарантирует, что nil-slice сериализуется как [], а не null.
// БД-колонки лицензий/СРО declared NOT NULL DEFAULT '[]'::jsonb.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

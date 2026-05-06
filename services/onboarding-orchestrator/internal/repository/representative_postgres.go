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

type PostgresRepresentativeRepository struct {
	db *sql.DB
}

func NewPostgresRepresentativeRepository(db *sql.DB) *PostgresRepresentativeRepository {
	return &PostgresRepresentativeRepository{db: db}
}

func (r *PostgresRepresentativeRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresRepresentativeRepository) Upsert(ctx context.Context, rep *domain.Representative) error {
	tx, err := r.withTenantTx(ctx, rep.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if rep.CreatedAt.IsZero() {
		rep.CreatedAt = now
	}
	rep.UpdatedAt = now

	idDocJSON, _ := json.Marshal(rep.IDDocument)
	authJSON, _ := json.Marshal(rep.Authority)
	regAddrJSON, _ := jsonMarshal(rep.RegistrationAddress)
	actAddrJSON, _ := jsonMarshal(rep.ActualAddress)
	foreignJSON, _ := jsonMarshal(rep.ForeignerInfo)
	pdlJSON, _ := jsonMarshal(rep.PDLDeclaration)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO representatives (
			id, tenant_id, application_id, legal_entity_id,
			last_name, first_name, middle_name,
			birth_date, birth_place, citizenship,
			inn, snils, id_document,
			registration_address, actual_address, foreigner_info,
			authority, pdl_declaration,
			is_primary, is_signatory,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,NULLIF($7,''),
			$8::date, NULLIF($9,''), $10,
			NULLIF($11,''), NULLIF($12,''), $13,
			$14, $15, $16,
			$17, $18,
			$19, $20,
			$21, $22
		)
		ON CONFLICT (id) DO UPDATE SET
			last_name             = EXCLUDED.last_name,
			first_name            = EXCLUDED.first_name,
			middle_name           = EXCLUDED.middle_name,
			birth_date            = EXCLUDED.birth_date,
			birth_place           = EXCLUDED.birth_place,
			citizenship           = EXCLUDED.citizenship,
			inn                   = EXCLUDED.inn,
			snils                 = EXCLUDED.snils,
			id_document           = EXCLUDED.id_document,
			registration_address  = EXCLUDED.registration_address,
			actual_address        = EXCLUDED.actual_address,
			foreigner_info        = EXCLUDED.foreigner_info,
			authority             = EXCLUDED.authority,
			pdl_declaration       = EXCLUDED.pdl_declaration,
			is_primary            = EXCLUDED.is_primary,
			is_signatory          = EXCLUDED.is_signatory,
			updated_at            = EXCLUDED.updated_at`,
		rep.ID, rep.TenantID, rep.ApplicationID, rep.LegalEntityID,
		rep.LastName, rep.FirstName, rep.MiddleName,
		rep.BirthDate, rep.BirthPlace, pq.Array(orEmpty(rep.Citizenship)),
		rep.INN, rep.SNILS, idDocJSON,
		regAddrJSON, actAddrJSON, foreignJSON,
		authJSON, pdlJSON,
		rep.IsPrimary, rep.IsSignatory,
		rep.CreatedAt, rep.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert representative: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresRepresentativeRepository) ListByApplication(ctx context.Context, tenantID, applicationID string) ([]*domain.Representative, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, repSelectColumns+` FROM representatives WHERE application_id = $1 ORDER BY is_primary DESC, created_at ASC`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*domain.Representative, 0)
	for rows.Next() {
		rep, err := scanRepresentative(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepresentativeRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Representative, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, repSelectColumns+` FROM representatives WHERE id = $1`, id)
	rep, err := scanRepresentative(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return rep, nil
}

const repSelectColumns = `
	SELECT id, tenant_id, application_id, legal_entity_id,
	       last_name, first_name, COALESCE(middle_name,''),
	       birth_date, COALESCE(birth_place,''), citizenship,
	       COALESCE(inn,''), COALESCE(snils,''), id_document,
	       registration_address, actual_address, foreigner_info,
	       authority, pdl_declaration,
	       is_primary, is_signatory,
	       created_at, updated_at`

func scanRepresentative(s rowScanner) (*domain.Representative, error) {
	var (
		rep                                      domain.Representative
		citizenship                              pq.StringArray
		idDocJSON, authJSON                       []byte
		regAddrJSON, actAddrJSON, foreignJSON     []byte
		pdlJSON                                   []byte
		birthDate                                 time.Time
	)
	if err := s.Scan(
		&rep.ID, &rep.TenantID, &rep.ApplicationID, &rep.LegalEntityID,
		&rep.LastName, &rep.FirstName, &rep.MiddleName,
		&birthDate, &rep.BirthPlace, &citizenship,
		&rep.INN, &rep.SNILS, &idDocJSON,
		&regAddrJSON, &actAddrJSON, &foreignJSON,
		&authJSON, &pdlJSON,
		&rep.IsPrimary, &rep.IsSignatory,
		&rep.CreatedAt, &rep.UpdatedAt,
	); err != nil {
		return nil, err
	}
	rep.BirthDate = birthDate.Format("2006-01-02")
	rep.Citizenship = []string(citizenship)
	if len(idDocJSON) > 0 {
		if err := json.Unmarshal(idDocJSON, &rep.IDDocument); err != nil {
			return nil, fmt.Errorf("decode id_document: %w", err)
		}
	}
	if len(authJSON) > 0 {
		if err := json.Unmarshal(authJSON, &rep.Authority); err != nil {
			return nil, fmt.Errorf("decode authority: %w", err)
		}
	}
	if err := jsonUnmarshalNullable(regAddrJSON, &rep.RegistrationAddress); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(actAddrJSON, &rep.ActualAddress); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(foreignJSON, &rep.ForeignerInfo); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalNullable(pdlJSON, &rep.PDLDeclaration); err != nil {
		return nil, err
	}
	return &rep, nil
}

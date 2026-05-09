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

type PostgresScreeningRepository struct {
	db *sql.DB
}

func NewPostgresScreeningRepository(db *sql.DB) *PostgresScreeningRepository {
	return &PostgresScreeningRepository{db: db}
}

func (r *PostgresScreeningRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresScreeningRepository) Upsert(ctx context.Context, s *domain.ScreeningResultSet) error {
	tx, err := r.withTenantTx(ctx, s.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.PerformedAt.IsZero() {
		s.PerformedAt = now
	}

	sanctionsJSON, _ := json.Marshal(orEmpty(s.SanctionsResults))
	pepJSON, _ := json.Marshal(orEmpty(s.PEPResults))
	mediaJSON, _ := json.Marshal(orEmpty(s.AdverseMediaHits))
	antiFraudJSON, _ := jsonMarshal(s.AntiFraudSignals)

	var okvedScore, turnoverScore sql.NullFloat64
	if s.OKVEDConsistencyScore != nil {
		okvedScore = sql.NullFloat64{Float64: *s.OKVEDConsistencyScore, Valid: true}
	}
	if s.TurnoverRealismScore != nil {
		turnoverScore = sql.NullFloat64{Float64: *s.TurnoverRealismScore, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO screening_result_sets (
			id, tenant_id, application_id,
			sanctions_results, pep_results, adverse_media_hits,
			okved_consistency_score, turnover_realism_score, anti_fraud_signals,
			performed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (application_id) DO UPDATE SET
			sanctions_results        = EXCLUDED.sanctions_results,
			pep_results              = EXCLUDED.pep_results,
			adverse_media_hits       = EXCLUDED.adverse_media_hits,
			okved_consistency_score  = EXCLUDED.okved_consistency_score,
			turnover_realism_score   = EXCLUDED.turnover_realism_score,
			anti_fraud_signals       = EXCLUDED.anti_fraud_signals,
			performed_at             = EXCLUDED.performed_at,
			updated_at               = EXCLUDED.updated_at`,
		s.ID, s.TenantID, s.ApplicationID,
		sanctionsJSON, pepJSON, mediaJSON,
		okvedScore, turnoverScore, antiFraudJSON,
		s.PerformedAt, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert screening_result_set: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresScreeningRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.ScreeningResultSet, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id,
		       sanctions_results, pep_results, adverse_media_hits,
		       okved_consistency_score, turnover_realism_score, anti_fraud_signals,
		       performed_at, created_at, updated_at
		FROM screening_result_sets
		WHERE application_id = $1`, applicationID)

	var (
		s              domain.ScreeningResultSet
		sanctionsJSON  []byte
		pepJSON        []byte
		mediaJSON      []byte
		antiFraudJSON  []byte
		okvedScore     sql.NullFloat64
		turnoverScore  sql.NullFloat64
	)
	if err := row.Scan(
		&s.ID, &s.TenantID, &s.ApplicationID,
		&sanctionsJSON, &pepJSON, &mediaJSON,
		&okvedScore, &turnoverScore, &antiFraudJSON,
		&s.PerformedAt, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan screening: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if okvedScore.Valid {
		v := okvedScore.Float64
		s.OKVEDConsistencyScore = &v
	}
	if turnoverScore.Valid {
		v := turnoverScore.Float64
		s.TurnoverRealismScore = &v
	}
	if len(sanctionsJSON) > 0 {
		if err := json.Unmarshal(sanctionsJSON, &s.SanctionsResults); err != nil {
			return nil, fmt.Errorf("decode sanctions: %w", err)
		}
	}
	if len(pepJSON) > 0 {
		if err := json.Unmarshal(pepJSON, &s.PEPResults); err != nil {
			return nil, fmt.Errorf("decode pep: %w", err)
		}
	}
	if len(mediaJSON) > 0 {
		if err := json.Unmarshal(mediaJSON, &s.AdverseMediaHits); err != nil {
			return nil, fmt.Errorf("decode adverse_media: %w", err)
		}
	}
	if err := jsonUnmarshalNullable(antiFraudJSON, &s.AntiFraudSignals); err != nil {
		return nil, err
	}
	return &s, nil
}

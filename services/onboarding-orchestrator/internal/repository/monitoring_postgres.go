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

type PostgresMonitoringRepository struct {
	db *sql.DB
}

func NewPostgresMonitoringRepository(db *sql.DB) *PostgresMonitoringRepository {
	return &PostgresMonitoringRepository{db: db}
}

func (r *PostgresMonitoringRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresMonitoringRepository) Upsert(ctx context.Context, p *domain.MonitoringProfile) error {
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

	rulesJSON, _ := json.Marshal(orEmpty(p.MonitoringRules))
	limitsJSON, _ := jsonMarshal(p.TransactionLimits)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO monitoring_profiles (
			id, tenant_id, application_id, account_id,
			review_frequency_months, next_review_date,
			monitoring_rules, kyc_refresh_triggers,
			transaction_limits, notification_channels,
			created_at, updated_at
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6::date,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (application_id) DO UPDATE SET
			account_id               = EXCLUDED.account_id,
			review_frequency_months  = EXCLUDED.review_frequency_months,
			next_review_date         = EXCLUDED.next_review_date,
			monitoring_rules         = EXCLUDED.monitoring_rules,
			kyc_refresh_triggers     = EXCLUDED.kyc_refresh_triggers,
			transaction_limits       = EXCLUDED.transaction_limits,
			notification_channels    = EXCLUDED.notification_channels,
			updated_at               = EXCLUDED.updated_at`,
		p.ID, p.TenantID, p.ApplicationID, p.AccountID,
		p.ReviewFrequencyMonths, p.NextReviewDate,
		rulesJSON, pq.Array(orEmpty(p.KYCRefreshTriggers)),
		limitsJSON, pq.Array(orEmpty(p.NotificationChannels)),
		p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert monitoring_profile: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresMonitoringRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.MonitoringProfile, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id, COALESCE(account_id,''),
		       review_frequency_months, next_review_date,
		       monitoring_rules, kyc_refresh_triggers,
		       transaction_limits, notification_channels,
		       created_at, updated_at
		FROM monitoring_profiles
		WHERE application_id = $1`, applicationID)

	var (
		p             domain.MonitoringProfile
		nextReview    time.Time
		rulesJSON     []byte
		triggers      pq.StringArray
		limitsJSON    []byte
		channels      pq.StringArray
	)
	if err := row.Scan(
		&p.ID, &p.TenantID, &p.ApplicationID, &p.AccountID,
		&p.ReviewFrequencyMonths, &nextReview,
		&rulesJSON, &triggers, &limitsJSON, &channels,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan monitoring_profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	p.NextReviewDate = nextReview.Format("2006-01-02")
	p.KYCRefreshTriggers = []string(triggers)
	p.NotificationChannels = []string(channels)
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &p.MonitoringRules); err != nil {
			return nil, fmt.Errorf("decode rules: %w", err)
		}
	}
	if err := jsonUnmarshalNullable(limitsJSON, &p.TransactionLimits); err != nil {
		return nil, err
	}
	return &p, nil
}

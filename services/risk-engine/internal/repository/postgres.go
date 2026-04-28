package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

// PostgresRiskAssessmentRepository persists assessments per-tenant via the
// schema-per-tenant strategy from ADR-0002. Schema is set on each call via
// `SET LOCAL search_path` inside a short-lived transaction so connections
// return to the pool clean.
type PostgresRiskAssessmentRepository struct {
	db *sql.DB
}

// NewPostgresRiskAssessmentRepository wires the DB pool into a repo.
func NewPostgresRiskAssessmentRepository(db *sql.DB) *PostgresRiskAssessmentRepository {
	return &PostgresRiskAssessmentRepository{db: db}
}

// validTenantID — same whitelist as tenant-service to keep schema lookup safe.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q", tenantID)
	}
	return "tnt_" + tenantID, nil
}

// withTenantTx runs fn inside a transaction with `SET LOCAL search_path` so
// every query sees only the target tenant's schema. The transaction is the
// boundary — `SET LOCAL` is reverted automatically on commit/rollback.
func (r *PostgresRiskAssessmentRepository) withTenantTx(
	ctx context.Context, tenantID string, fn func(*sql.Tx) error,
) error {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Schema name is regex-validated; safe to interpolate after QuoteIdentifier
	// would still need the same whitelist, so we keep it simple.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`SET LOCAL search_path TO %q, public`, schema),
	); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Create writes a new RiskAssessment into the tenant schema.
func (r *PostgresRiskAssessmentRepository) Create(ctx context.Context, a *domain.RiskAssessment) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	a.UpdatedAt = a.CreatedAt

	factorsJSON, err := json.Marshal(a.Factors)
	if err != nil {
		return fmt.Errorf("marshal factors: %w", err)
	}
	rulesJSON, err := json.Marshal(a.RulesTriggered)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	screeningJSON, err := json.Marshal(a.ScreeningResults)
	if err != nil {
		return fmt.Errorf("marshal screening: %w", err)
	}
	modelJSON, err := json.Marshal(a.Model)
	if err != nil {
		return fmt.Errorf("marshal model: %w", err)
	}
	explanationJSON, err := json.Marshal(a.Explanation)
	if err != nil {
		return fmt.Errorf("marshal explanation: %w", err)
	}

	return r.withTenantTx(ctx, a.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO risk_assessments
			  (id, application_id, legal_entity_id, score, category,
			   model, factors, rules_triggered, screening_results,
			   explanation, recommendation, created_at, updated_at)
			VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			a.ID, a.ApplicationID, a.LegalEntityID, a.Score, string(a.Category),
			modelJSON, factorsJSON, rulesJSON, screeningJSON,
			explanationJSON, string(a.Recommendation),
			a.CreatedAt, a.UpdatedAt,
		)
		return err
	})
}

// GetByID returns one assessment from the tenant schema or domain.ErrNotFound.
func (r *PostgresRiskAssessmentRepository) GetByID(
	ctx context.Context, tenantID, id string,
) (*domain.RiskAssessment, error) {
	var a *domain.RiskAssessment
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT id, application_id, COALESCE(legal_entity_id, ''),
			       score, category,
			       model, factors, rules_triggered, screening_results,
			       explanation, recommendation, created_at, updated_at
			FROM risk_assessments WHERE id = $1`, id)
		got, scanErr := scanAssessment(row, tenantID)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if scanErr != nil {
			return scanErr
		}
		a = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListByApplication returns assessments for a single application, newest first.
func (r *PostgresRiskAssessmentRepository) ListByApplication(
	ctx context.Context, tenantID, applicationID string,
) ([]*domain.RiskAssessment, error) {
	out := make([]*domain.RiskAssessment, 0)
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, application_id, COALESCE(legal_entity_id, ''),
			       score, category,
			       model, factors, rules_triggered, screening_results,
			       explanation, recommendation, created_at, updated_at
			FROM risk_assessments
			WHERE application_id = $1
			ORDER BY created_at DESC`, applicationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAssessment(rows, tenantID)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAssessment(s rowScanner, tenantID string) (*domain.RiskAssessment, error) {
	var (
		a                                                     domain.RiskAssessment
		category, recommendation                              string
		modelJSON, factorsJSON, rulesJSON, screeningJSON, expJSON []byte
	)
	if err := s.Scan(
		&a.ID, &a.ApplicationID, &a.LegalEntityID, &a.Score, &category,
		&modelJSON, &factorsJSON, &rulesJSON, &screeningJSON, &expJSON,
		&recommendation, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	a.TenantID = tenantID
	a.Category = domain.RiskCategory(category)
	a.Recommendation = domain.Recommendation(recommendation)
	if err := json.Unmarshal(modelJSON, &a.Model); err != nil {
		return nil, fmt.Errorf("decode model: %w", err)
	}
	if err := json.Unmarshal(factorsJSON, &a.Factors); err != nil {
		return nil, fmt.Errorf("decode factors: %w", err)
	}
	if err := json.Unmarshal(rulesJSON, &a.RulesTriggered); err != nil {
		return nil, fmt.Errorf("decode rules: %w", err)
	}
	if err := json.Unmarshal(screeningJSON, &a.ScreeningResults); err != nil {
		return nil, fmt.Errorf("decode screening: %w", err)
	}
	if err := json.Unmarshal(expJSON, &a.Explanation); err != nil {
		return nil, fmt.Errorf("decode explanation: %w", err)
	}
	return &a, nil
}

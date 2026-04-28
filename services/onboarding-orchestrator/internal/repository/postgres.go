// Package repository — Postgres-имплементации портов из internal/domain.
//
// Изоляция тенантов — через search_path (ADR-0002): каждое подключение
// перед запросом устанавливает SET LOCAL search_path TO tnt_<id>, public.
// Имена схем формируются после жёсткой валидации tenantID (regex).
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/lib/pq"

	"aibank/onboarding-orchestrator/internal/domain"
)

// ErrNotFound возвращается, когда запись не найдена.
var ErrNotFound = errors.New("application not found")

// ErrInvalidTransition — попытка перевести заявку в недопустимое состояние.
// См. ApplicationState state machine в internal/domain/state.go.
var ErrInvalidTransition = errors.New("invalid state transition")

// validTenantID — узкий whitelist для частей идентификатора схемы.
// Совпадает с правилом в tenant-service/internal/repository/postgres.go,
// чтобы tenant_id оставался согласованным во всей платформе.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q", tenantID)
	}
	return "tnt_" + tenantID, nil
}

// PostgresApplicationRepository реализует domain.ApplicationRepository.
type PostgresApplicationRepository struct {
	db *sql.DB
}

func NewPostgresApplicationRepository(db *sql.DB) *PostgresApplicationRepository {
	return &PostgresApplicationRepository{db: db}
}

// withTenantTx открывает транзакцию и выставляет search_path для тенанта.
// SET LOCAL — действие ограничено транзакцией, соединение возвращается в
// пул в чистом состоянии (см. ADR-0002).
func (r *PostgresApplicationRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
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

func (r *PostgresApplicationRepository) Create(ctx context.Context, app *domain.Application) error {
	if !app.State.IsValid() {
		return fmt.Errorf("invalid initial state %q", app.State)
	}
	now := time.Now().UTC()
	if app.CreatedAt.IsZero() {
		app.CreatedAt = now
	}
	app.UpdatedAt = now

	tx, err := r.withTenantTx(ctx, app.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO applications (
			id, tenant_id, applicant_id, legal_entity_id, legal_entity_type,
			channel, state, product_codes, workflow_id,
			risk_assessment_id, decision_id, account_ids,
			created_at, updated_at
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''),$12,$13,$14)`,
		app.ID, app.TenantID, app.ApplicantID, app.LegalEntityID, app.LegalEntityType,
		app.Channel, app.State, pq.Array(app.ProductCodes), app.WorkflowID,
		app.RiskAssessmentID, app.DecisionID, pq.Array(app.AccountIDs),
		app.CreatedAt, app.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert application: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresApplicationRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Application, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	app, err := scanOne(ctx, tx, `
		SELECT id, tenant_id, applicant_id, COALESCE(legal_entity_id,''), legal_entity_type,
		       channel, state, product_codes, workflow_id,
		       COALESCE(risk_assessment_id,''), COALESCE(decision_id,''), account_ids,
		       created_at, updated_at, completed_at, archived_at
		FROM applications WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return app, nil
}

func (r *PostgresApplicationRepository) UpdateState(ctx context.Context, tenantID, id string, newState domain.ApplicationState) error {
	if !newState.IsValid() {
		return fmt.Errorf("invalid target state %q", newState)
	}

	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var current domain.ApplicationState
	err = tx.QueryRowContext(ctx, `SELECT state FROM applications WHERE id = $1 FOR UPDATE`, id).
		Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load current state: %w", err)
	}

	if !domain.CanTransition(current, newState) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, current, newState)
	}

	now := time.Now().UTC()
	completedAt := sql.NullTime{}
	if newState.IsTerminal() {
		completedAt = sql.NullTime{Time: now, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE applications
		   SET state = $2, updated_at = $3,
		       completed_at = COALESCE(completed_at, $4)
		 WHERE id = $1`, id, newState, now, completedAt)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresApplicationRepository) ListByTenant(ctx context.Context, tenantID string, limit int) ([]*domain.Application, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, tenant_id, applicant_id, COALESCE(legal_entity_id,''), legal_entity_type,
		       channel, state, product_codes, workflow_id,
		       COALESCE(risk_assessment_id,''), COALESCE(decision_id,''), account_ids,
		       created_at, updated_at, completed_at, archived_at
		  FROM applications
		 ORDER BY created_at DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*domain.Application, 0)
	for rows.Next() {
		app, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, app)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// rowScanner — общий интерфейс для *sql.Row и *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanOne(ctx context.Context, tx *sql.Tx, query string, args ...any) (*domain.Application, error) {
	row := tx.QueryRowContext(ctx, query, args...)
	app, err := scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return app, err
}

func scanRow(s rowScanner) (*domain.Application, error) {
	var (
		app          domain.Application
		productCodes pq.StringArray
		accountIDs   pq.StringArray
		completedAt  sql.NullTime
		archivedAt   sql.NullTime
	)
	if err := s.Scan(
		&app.ID, &app.TenantID, &app.ApplicantID, &app.LegalEntityID, &app.LegalEntityType,
		&app.Channel, &app.State, &productCodes, &app.WorkflowID,
		&app.RiskAssessmentID, &app.DecisionID, &accountIDs,
		&app.CreatedAt, &app.UpdatedAt, &completedAt, &archivedAt,
	); err != nil {
		return nil, err
	}
	app.ProductCodes = []string(productCodes)
	app.AccountIDs = []string(accountIDs)
	if completedAt.Valid {
		t := completedAt.Time
		app.CompletedAt = &t
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		app.ArchivedAt = &t
	}
	return &app, nil
}

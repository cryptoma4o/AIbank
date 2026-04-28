// Package runner применяет миграции к platform-схемам или к схеме конкретного
// тенанта согласно ADR-0005. Не пишет down-миграции — политика репо
// "production = forward fix only" зафиксирована в ADR-0005.
package runner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/aibank/platform/tools/db-migrator/internal/source"
)

// PlatformRunner применяет миграции к platform-уровню (схема `platform`,
// `audit`, `temporal_visibility` и т.п.). Версии хранятся в
// `platform_meta.schema_migrations`.
type PlatformRunner struct {
	db  *sql.DB
	log *slog.Logger
}

func NewPlatformRunner(db *sql.DB, log *slog.Logger) *PlatformRunner {
	return &PlatformRunner{db: db, log: log}
}

// Apply применяет миграции, пропуская уже применённые версии.
// При несовпадении checksum уже применённой миграции — ошибка
// (защита от изменения файла после применения).
func (r *PlatformRunner) Apply(ctx context.Context, migs []source.Migration) error {
	if err := r.ensureRegistry(ctx); err != nil {
		return fmt.Errorf("ensure registry: %w", err)
	}
	for _, m := range migs {
		applied, storedSum, err := r.applyState(ctx, m.Version)
		if err != nil {
			return err
		}
		if applied {
			if storedSum != m.Checksum {
				return fmt.Errorf(
					"checksum drift for version %d (%s): stored=%s, file=%s",
					m.Version, m.Name, storedSum, m.Checksum)
			}
			r.log.Info("skip already applied", "version", m.Version, "name", m.Name)
			continue
		}
		r.log.Info("applying", "version", m.Version, "name", m.Name)
		if err := r.applyOne(ctx, m); err != nil {
			return fmt.Errorf("apply %d_%s: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

func (r *PlatformRunner) ensureRegistry(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS platform_meta;
		CREATE TABLE IF NOT EXISTS platform_meta.schema_migrations (
			version    INT PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			checksum   TEXT NOT NULL
		);
	`)
	return err
}

func (r *PlatformRunner) applyState(ctx context.Context, version int) (bool, string, error) {
	var checksum string
	err := r.db.QueryRowContext(ctx,
		`SELECT checksum FROM platform_meta.schema_migrations WHERE version = $1`,
		version).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, checksum, nil
}

func (r *PlatformRunner) applyOne(ctx context.Context, m source.Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.UpSQL); err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO platform_meta.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		m.Version, m.Name, m.Checksum); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

// TenantRunner применяет миграции к схеме конкретного тенанта (`tnt_<id>`)
// и записывает применённые версии в `platform.tenant_migrations`.
type TenantRunner struct {
	db          *sql.DB
	log         *slog.Logger
	serviceName string
	runID       string
}

func NewTenantRunner(db *sql.DB, log *slog.Logger, serviceName, runID string) *TenantRunner {
	return &TenantRunner{db: db, log: log, serviceName: serviceName, runID: runID}
}

// validTenantID — тот же whitelist, что в tenant-service (ADR-0002).
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func (r *TenantRunner) Apply(ctx context.Context, tenantID string, migs []source.Migration) error {
	if !validTenantID.MatchString(tenantID) {
		return fmt.Errorf("invalid tenant id %q", tenantID)
	}
	schema := "tnt_" + tenantID

	for _, m := range migs {
		applied, storedSum, err := r.applyState(ctx, tenantID, m.Version)
		if err != nil {
			return err
		}
		if applied {
			if storedSum != m.Checksum {
				return fmt.Errorf(
					"checksum drift for tenant %s version %d (%s): stored=%s, file=%s",
					tenantID, m.Version, m.Name, storedSum, m.Checksum)
			}
			r.log.Info("skip already applied", "tenant", tenantID,
				"service", r.serviceName, "version", m.Version)
			continue
		}
		r.log.Info("applying tenant migration",
			"tenant", tenantID, "service", r.serviceName,
			"version", m.Version, "name", m.Name)
		if err := r.applyOne(ctx, tenantID, schema, m); err != nil {
			return fmt.Errorf("apply tenant %s version %d: %w", tenantID, m.Version, err)
		}
	}
	return nil
}

func (r *TenantRunner) applyState(ctx context.Context, tenantID string, version int) (bool, string, error) {
	var checksum string
	err := r.db.QueryRowContext(ctx,
		`SELECT checksum FROM platform.tenant_migrations
		 WHERE tenant_id = $1 AND service_name = $2 AND version = $3`,
		tenantID, r.serviceName, version).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, checksum, nil
}

func (r *TenantRunner) applyOne(ctx context.Context, tenantID, schema string, m source.Migration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// search_path устанавливаем через set_config с параметризацией —
	// безопаснее, чем интерполировать имя схемы в текст SQL.
	if _, err := tx.ExecContext(ctx,
		`SELECT set_config('search_path', $1, true)`, schema); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if _, err := tx.ExecContext(ctx, m.UpSQL); err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO platform.tenant_migrations
		   (tenant_id, service_name, version, applied_by, checksum)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, r.serviceName, m.Version, r.runID, m.Checksum); err != nil {
		return fmt.Errorf("record tenant migration: %w", err)
	}
	return tx.Commit()
}

// ListActiveTenants возвращает идентификаторы активных тенантов
// (`status` ∈ {trial, active}). Для use-case `tenant-all`.
func ListActiveTenants(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id FROM platform.tenants
		 WHERE status IN ('trial', 'active')
		 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

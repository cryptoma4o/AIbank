package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aibank/platform/services/tenant-service/internal/domain"
)

// ErrNotFound возвращается, когда сущность не найдена.
var ErrNotFound = errors.New("not found")

type PostgresTenantRepository struct {
	db *sql.DB
}

func NewPostgresTenantRepository(db *sql.DB) *PostgresTenantRepository {
	return &PostgresTenantRepository{db: db}
}

func (r *PostgresTenantRepository) GetByID(ctx context.Context, id string) (*domain.Tenant, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, bik, inn, status, deployment_mode, created_at, updated_at
		 FROM platform.tenants WHERE id = $1`, id)

	var t domain.Tenant
	err := row.Scan(&t.ID, &t.Name, &t.BIK, &t.INN, &t.Status, &t.DeploymentMode, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PostgresTenantRepository) List(ctx context.Context) ([]*domain.Tenant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, bik, inn, status, deployment_mode, created_at, updated_at
		 FROM platform.tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenants := make([]*domain.Tenant, 0)
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
	res, err := r.db.ExecContext(ctx,
		`UPDATE platform.tenants SET name=$2, status=$3, deployment_mode=$4, updated_at=$5 WHERE id=$1`,
		t.ID, t.Name, t.Status, t.DeploymentMode, t.UpdatedAt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PostgresTenantConfigRepository реализует TenantConfigRepository.
type PostgresTenantConfigRepository struct {
	db *sql.DB
}

func NewPostgresTenantConfigRepository(db *sql.DB) *PostgresTenantConfigRepository {
	return &PostgresTenantConfigRepository{db: db}
}

func (r *PostgresTenantConfigRepository) GetConfig(ctx context.Context, tenantID string) (*domain.TenantConfig, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT tenant_id, raw_config, schema_version, updated_at
		 FROM platform.tenant_configs WHERE tenant_id = $1`, tenantID)
	var c domain.TenantConfig
	err := row.Scan(&c.TenantID, &c.RawConfig, &c.SchemaVersion, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresTenantConfigRepository) SaveConfig(ctx context.Context, cfg *domain.TenantConfig) error {
	cfg.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO platform.tenant_configs (tenant_id, raw_config, schema_version, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id) DO UPDATE
		   SET raw_config = EXCLUDED.raw_config,
		       schema_version = EXCLUDED.schema_version,
		       updated_at = EXCLUDED.updated_at`,
		cfg.TenantID, cfg.RawConfig, cfg.SchemaVersion, cfg.UpdatedAt)
	return err
}

// PostgresSchemaProvisioner реализует SchemaProvisioner для PostgreSQL.
//
// Имя схемы формируется как `tnt_<tenantID>` после жёсткой валидации tenantID
// (защита от SQL-injection через идентификатор — см. ADR-0002, раздел Безопасность).
type PostgresSchemaProvisioner struct {
	db *sql.DB
}

func NewPostgresSchemaProvisioner(db *sql.DB) *PostgresSchemaProvisioner {
	return &PostgresSchemaProvisioner{db: db}
}

// validTenantID — узкий whitelist для частей идентификатора схемы.
// Соответствует требованиям PostgreSQL identifier syntax + ADR-0002.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q: must match %s", tenantID, validTenantID)
	}
	return "tnt_" + tenantID, nil
}

func (p *PostgresSchemaProvisioner) SchemaExists(ctx context.Context, tenantID string) (bool, error) {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return false, err
	}
	var exists bool
	err = p.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`,
		schema).Scan(&exists)
	return exists, err
}

func (p *PostgresSchemaProvisioner) Provision(ctx context.Context, tenantID string) error {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return err
	}

	// Идентификатор уже валидирован regex'ом — безопасно интерполировать.
	createStmt := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %q`, schema)
	if _, err := p.db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create schema %s: %w", schema, err)
	}

	// Роль приложения тенанта. Не выдаём пароль здесь — пароли управляются Vault'ом
	// и инжектятся в DSN сервисами при старте.
	roleApp := schema + "_app"
	createRole := fmt.Sprintf(
		`DO $$
		 BEGIN
		   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '%[1]s') THEN
		     CREATE ROLE %[2]q NOLOGIN;
		   END IF;
		 END$$`, roleApp, roleApp)
	if _, err := p.db.ExecContext(ctx, createRole); err != nil {
		return fmt.Errorf("create role %s: %w", roleApp, err)
	}

	grants := fmt.Sprintf(
		`GRANT USAGE ON SCHEMA %q TO %q;
		 ALTER DEFAULT PRIVILEGES IN SCHEMA %q
		   GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %q;`,
		schema, roleApp, schema, roleApp)
	if _, err := p.db.ExecContext(ctx, grants); err != nil {
		return fmt.Errorf("grant on schema %s: %w", schema, err)
	}

	return nil
}

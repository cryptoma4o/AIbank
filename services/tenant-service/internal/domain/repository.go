package domain

import "context"

type TenantRepository interface {
	GetByID(ctx context.Context, id string) (*Tenant, error)
	List(ctx context.Context) ([]*Tenant, error)
	Create(ctx context.Context, t *Tenant) error
	Update(ctx context.Context, t *Tenant) error
}

type TenantConfigRepository interface {
	GetConfig(ctx context.Context, tenantID string) (*TenantConfig, error)
	SaveConfig(ctx context.Context, cfg *TenantConfig) error
}

// SchemaProvisioner создаёт изолированную PostgreSQL-схему для нового тенанта
// согласно ADR-0002 (schema-per-tenant). Применение DDL-миграций к схеме —
// ответственность db-migrator job (ADR-0005); провижионер только готовит
// пустую схему с корректными правами доступа.
type SchemaProvisioner interface {
	// Provision создаёт schema tnt_<tenantID> и роль tnt_<tenantID>_app
	// с грантом USAGE на схему. Идемпотентно: повторный вызов на готовый
	// тенант возвращает nil без эффекта.
	Provision(ctx context.Context, tenantID string) error

	// SchemaExists проверяет наличие схемы тенанта.
	SchemaExists(ctx context.Context, tenantID string) (bool, error)
}

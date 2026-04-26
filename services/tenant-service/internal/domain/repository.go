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

-- +goose Up
CREATE TABLE IF NOT EXISTS clients (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    inn        TEXT NOT NULL,
    ogrn       TEXT NOT NULL,
    full_name  TEXT NOT NULL,
    type       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_inn_tenant ON clients(inn, tenant_id);
CREATE INDEX IF NOT EXISTS idx_clients_tenant ON clients(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS clients;

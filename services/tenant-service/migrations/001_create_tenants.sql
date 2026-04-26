-- +goose Up
CREATE SCHEMA IF NOT EXISTS platform;

CREATE TABLE platform.tenants (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    bik             CHAR(9) NOT NULL,
    inn             CHAR(10) NOT NULL,
    status          TEXT NOT NULL DEFAULT 'trial'
                        CHECK (status IN ('trial','active','suspended','terminated')),
    deployment_mode TEXT NOT NULL
                        CHECK (deployment_mode IN ('saas','on_prem','hybrid')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE platform.tenant_configs (
    tenant_id       TEXT PRIMARY KEY REFERENCES platform.tenants(id),
    raw_config      BYTEA NOT NULL,
    schema_version  TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS platform.tenant_configs;
DROP TABLE IF EXISTS platform.tenants;
DROP SCHEMA IF EXISTS platform;

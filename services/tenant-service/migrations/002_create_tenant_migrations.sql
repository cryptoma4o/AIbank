-- +goose Up
--
-- Реестр применённых миграций per tenant per service.
-- Источник правды для db-migrator (ADR-0005): какие версии миграций
-- какого сервиса применены к схеме каждого тенанта.

CREATE TABLE platform.tenant_migrations (
    tenant_id    TEXT NOT NULL REFERENCES platform.tenants(id) ON DELETE CASCADE,
    service_name TEXT NOT NULL,                   -- 'tenant-service', 'audit-service', и т.д.
    version      INT  NOT NULL,
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_by   TEXT NOT NULL,                   -- run_id Temporal workflow или 'manual'
    checksum     TEXT NOT NULL,                   -- SHA-256 файла миграции
    PRIMARY KEY (tenant_id, service_name, version)
);

CREATE INDEX idx_tenant_migrations_service ON platform.tenant_migrations (service_name, version);

-- +goose Down
DROP TABLE IF EXISTS platform.tenant_migrations;

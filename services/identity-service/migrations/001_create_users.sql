-- +goose Up
-- platform.users: глобальная таблица пользователей платформы и банковских операторов.
-- ADR-0002: схема `platform` живёт в общем кластере, изолирована от tnt_<id>.
-- tenant_id NULL допустим для платформенных ролей (platform.admin); для банковских
-- ролей CHECK ниже требует non-null значения.
CREATE SCHEMA IF NOT EXISTS platform;

CREATE TABLE platform.users (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NULL REFERENCES platform.tenants(id),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL
                      CHECK (role IN (
                          'platform.admin',
                          'bank.operator',
                          'bank.compliance_officer',
                          'bank.admin'
                      )),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Платформенные роли — без tenant_id; банковские — обязательно с ним.
    CONSTRAINT users_tenant_role_consistency CHECK (
        (role = 'platform.admin' AND tenant_id IS NULL)
        OR (role <> 'platform.admin' AND tenant_id IS NOT NULL)
    )
);

CREATE INDEX idx_users_tenant ON platform.users (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_users_role   ON platform.users (role);

-- +goose Down
DROP TABLE IF EXISTS platform.users;

-- +goose Up
CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    principal_id TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    auth_method  TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_principal ON sessions(principal_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;

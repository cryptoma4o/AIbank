-- +goose Up
CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE audit.events (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    actor_id      TEXT NOT NULL,
    actor_type    TEXT NOT NULL CHECK (actor_type IN ('user','system','ai_agent')),
    payload       JSONB NOT NULL DEFAULT '{}',
    previous_hash TEXT NOT NULL DEFAULT '',
    hash          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only enforced via trigger: no UPDATE or DELETE allowed
CREATE OR REPLACE FUNCTION audit.prevent_modification()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit.events is append-only — UPDATE and DELETE are forbidden';
END;
$$;

CREATE TRIGGER no_update BEFORE UPDATE ON audit.events
    FOR EACH ROW EXECUTE FUNCTION audit.prevent_modification();

CREATE TRIGGER no_delete BEFORE DELETE ON audit.events
    FOR EACH ROW EXECUTE FUNCTION audit.prevent_modification();

CREATE INDEX idx_audit_events_tenant_entity ON audit.events (tenant_id, entity_type, entity_id);
CREATE INDEX idx_audit_events_tenant_time ON audit.events (tenant_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit.events;
DROP FUNCTION IF EXISTS audit.prevent_modification();
DROP SCHEMA IF EXISTS audit;

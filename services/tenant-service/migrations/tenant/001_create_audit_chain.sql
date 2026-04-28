-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id>.
-- Применяется через db-migrator (ADR-0005), не tenant-service сам по себе.
--
-- ВАЖНО: миграция использует current_schema() и НЕ хардкодит имя схемы.
-- Тем самым один и тот же файл применяется к любой схеме тенанта.

-- Per-tenant audit log (изолирован от platform.audit_platform).
CREATE TABLE audit_events (
    id            TEXT PRIMARY KEY,
    entity_type   TEXT NOT NULL,
    entity_id     TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    actor_id      TEXT NOT NULL,
    actor_type    TEXT NOT NULL CHECK (actor_type IN ('user','system','ai_agent','external_api')),
    payload       JSONB NOT NULL DEFAULT '{}',
    previous_hash TEXT NOT NULL DEFAULT '',
    hash          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only enforcement: запрещаем UPDATE и DELETE.
CREATE OR REPLACE FUNCTION prevent_audit_modification()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only — UPDATE and DELETE are forbidden';
END;
$$;

CREATE TRIGGER no_update BEFORE UPDATE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_modification();

CREATE TRIGGER no_delete BEFORE DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_modification();

CREATE INDEX idx_audit_events_entity ON audit_events (entity_type, entity_id);
CREATE INDEX idx_audit_events_time   ON audit_events (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_events;
DROP FUNCTION IF EXISTS prevent_audit_modification();

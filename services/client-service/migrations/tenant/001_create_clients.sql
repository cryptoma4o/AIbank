-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id> (ADR-0002).
-- Использует current_schema() и НЕ хардкодит имя схемы — db-migrator
-- (ADR-0005) проходит этим файлом по всем активным тенантам.

-- Карточка клиента (КУС). Источник правды о связи Application <-> LegalEntity
-- <-> текущее обслуживание; полные данные юрлица — в legal-entity-service.
CREATE TABLE clients (
    id              TEXT PRIMARY KEY,
    applicant_id    TEXT NOT NULL,
    legal_entity_id TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'onboarding'
                        CHECK (status IN ('onboarding','active','suspended','archived')),
    risk_category   TEXT NOT NULL
                        CHECK (risk_category IN ('LOW','MEDIUM','HIGH')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_clients_applicant     ON clients (applicant_id);
CREATE INDEX idx_clients_legal_entity  ON clients (legal_entity_id);
CREATE INDEX idx_clients_created_desc  ON clients (created_at DESC);

-- Append-only история карточки клиента. Источник — orchestrator + сам сервис
-- (status-changes). Поле `source` хранит ссылку на audit_event_id (или
-- произвольный URI операции) — без жёсткого FK между сервисами.
CREATE TABLE client_history (
    id          TEXT PRIMARY KEY,
    client_id   TEXT NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    event_type  TEXT NOT NULL,
    summary     TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_client_history_client_time
    ON client_history (client_id, created_at DESC);

-- Append-only enforcement: UPDATE и DELETE недопустимы. Совпадает по форме с
-- audit_events (см. tenant-service/migrations/tenant/001_create_audit_chain.sql).
CREATE OR REPLACE FUNCTION prevent_client_history_modification()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'client_history is append-only — UPDATE and DELETE are forbidden';
END;
$$;

CREATE TRIGGER no_update BEFORE UPDATE ON client_history
    FOR EACH ROW EXECUTE FUNCTION prevent_client_history_modification();

CREATE TRIGGER no_delete BEFORE DELETE ON client_history
    FOR EACH ROW EXECUTE FUNCTION prevent_client_history_modification();

-- +goose Down
DROP TABLE IF EXISTS client_history;
DROP FUNCTION IF EXISTS prevent_client_history_modification();
DROP TABLE IF EXISTS clients;

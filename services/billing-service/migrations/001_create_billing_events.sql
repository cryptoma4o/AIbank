-- +goose Up
-- ADR-0010 § 3: platform.billing_events — cross-tenant таблица в схеме `platform`,
-- НЕ tenant-schema. ADR-0002 разрешает cross-tenant сущности только в `platform`.
CREATE SCHEMA IF NOT EXISTS platform;

CREATE TABLE platform.billing_events (
    id                  UUID PRIMARY KEY,
    tenant_id           TEXT NOT NULL REFERENCES platform.tenants(id),
    event_type          TEXT NOT NULL,
    source_service      TEXT NOT NULL,
    source_event_id     TEXT NOT NULL,
    quantity            INT  NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price_kopecks  BIGINT NOT NULL DEFAULT 0 CHECK (unit_price_kopecks >= 0),
    total_kopecks       BIGINT NOT NULL DEFAULT 0 CHECK (total_kopecks >= 0),
    metadata            JSONB NOT NULL DEFAULT '{}',
    audit_event_id      TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    billed_at           TIMESTAMPTZ,
    CONSTRAINT billing_events_total_consistent
        CHECK (total_kopecks = quantity * unit_price_kopecks)
);

-- Индексы под типовые запросы (ADR-0010 § 3).
CREATE INDEX idx_billing_events_tenant_time
    ON platform.billing_events (tenant_id, created_at DESC);
CREATE INDEX idx_billing_events_tenant_type_time
    ON platform.billing_events (tenant_id, event_type, created_at);
CREATE INDEX idx_billing_events_unbilled
    ON platform.billing_events (tenant_id, created_at)
    WHERE billed_at IS NULL;

-- Append-only enforced триггером: запрещены DELETE и UPDATE кроме UPDATE (billed_at).
-- Этот единственный разрешённый UPDATE используется месячным invoicing-workflow
-- (ADR-0010 § 6) для пометки события как включённого в счёт.
CREATE OR REPLACE FUNCTION platform.billing_events_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'platform.billing_events is append-only — DELETE forbidden';
    END IF;

    -- Разрешён только UPDATE (billed_at): остальные колонки должны совпадать.
    IF NEW.id                 IS DISTINCT FROM OLD.id                 OR
       NEW.tenant_id          IS DISTINCT FROM OLD.tenant_id          OR
       NEW.event_type         IS DISTINCT FROM OLD.event_type         OR
       NEW.source_service     IS DISTINCT FROM OLD.source_service     OR
       NEW.source_event_id    IS DISTINCT FROM OLD.source_event_id    OR
       NEW.quantity           IS DISTINCT FROM OLD.quantity           OR
       NEW.unit_price_kopecks IS DISTINCT FROM OLD.unit_price_kopecks OR
       NEW.total_kopecks      IS DISTINCT FROM OLD.total_kopecks      OR
       NEW.metadata           IS DISTINCT FROM OLD.metadata           OR
       NEW.audit_event_id     IS DISTINCT FROM OLD.audit_event_id     OR
       NEW.created_at         IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION
            'platform.billing_events is append-only — only billed_at may be updated';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER no_delete BEFORE DELETE ON platform.billing_events
    FOR EACH ROW EXECUTE FUNCTION platform.billing_events_immutable();

CREATE TRIGGER no_update_except_billed_at BEFORE UPDATE ON platform.billing_events
    FOR EACH ROW EXECUTE FUNCTION platform.billing_events_immutable();

-- +goose Down
DROP TRIGGER IF EXISTS no_update_except_billed_at ON platform.billing_events;
DROP TRIGGER IF EXISTS no_delete ON platform.billing_events;
DROP FUNCTION IF EXISTS platform.billing_events_immutable();
DROP TABLE IF EXISTS platform.billing_events;

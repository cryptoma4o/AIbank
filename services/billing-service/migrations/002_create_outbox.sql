-- +goose Up
-- ADR-0010 § 2: transactional outbox для at-least-once-доставки billing-events
-- в Kafka topic platform.billing.events.
--
-- В этом сервисе outbox cross-tenant (как и сами billing_events): мы пишем в
-- platform.billing_outbox в одной транзакции с INSERT'ом в platform.billing_events.
-- Это семантически отличается от tenant-outbox в onboarding-orchestrator (там
-- outbox per-tenant, потому что доменная транзакция — внутри tnt_<id>); здесь же
-- сам владелец domain-таблицы — platform.

CREATE TABLE IF NOT EXISTS platform.billing_outbox (
    id              BIGSERIAL PRIMARY KEY,
    aggregate_type  TEXT        NOT NULL,           -- 'billing_event'
    aggregate_id    TEXT        NOT NULL,           -- billing_events.id (UUID)
    event_type      TEXT        NOT NULL,           -- 'account_opened.llc' и т.д.
    payload         JSONB       NOT NULL,           -- сериализованный domain.BillingEvent
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ
);

-- Partial-index по unpublished-строкам: relay не сканирует уже отгруженные
-- записи. Cleanup published-rows — отдельный cron, в MVP не реализован
-- (см. packages/outbox/README.md TODO).
CREATE INDEX IF NOT EXISTS idx_billing_outbox_unpublished
    ON platform.billing_outbox (id)
    WHERE published_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS platform.idx_billing_outbox_unpublished;
DROP TABLE IF EXISTS platform.billing_outbox;

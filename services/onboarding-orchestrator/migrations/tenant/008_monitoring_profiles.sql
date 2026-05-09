-- +goose Up
--
-- Этапы 8 и 10 формы онбординга — параметры постоянного мониторинга
-- открытого счёта. Соответствует packages/domain-model/schema.json
-- $defs.MonitoringProfile (v1.1.0). Создаётся одновременно с Account.
--
-- monitoring_rules / kyc_refresh_triggers / transaction_limits /
-- notification_channels хранятся как JSONB — heterogeneous structures без
-- query-сценариев "найти все профили с rule-code X".

CREATE TABLE monitoring_profiles (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    application_id           TEXT NOT NULL,
    account_id               TEXT,
    review_frequency_months  INTEGER NOT NULL CHECK (review_frequency_months IN (3, 6, 12)),
    next_review_date         DATE NOT NULL,
    monitoring_rules         JSONB NOT NULL DEFAULT '[]'::JSONB,
    kyc_refresh_triggers     TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    transaction_limits       JSONB,
    notification_channels    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один профиль на заявку (375-П — periodic review одного клиента).
CREATE UNIQUE INDEX uniq_monitoring_profiles_application
    ON monitoring_profiles (application_id);

-- Lookup для cron-задач periodic review (next_review_date <= NOW).
CREATE INDEX idx_monitoring_profiles_next_review
    ON monitoring_profiles (next_review_date);

-- +goose Down
DROP TABLE IF EXISTS monitoring_profiles;

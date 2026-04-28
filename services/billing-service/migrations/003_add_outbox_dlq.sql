-- +goose Up
-- DLQ для transactional outbox в billing-service (расширение миграции 002).
-- Per ADR-0010 § 2 + packages/outbox README "TODO: DLQ для poison messages".
--
-- Add-only миграция: не ломает существующие данные в platform.billing_outbox.
-- Новые столбцы получают defaults (attempts=0, остальные NULL), все имеющиеся
-- неопубликованные строки начинают цикл повторов с 0.
--
-- Поведение relay после применения:
--   * При успехе Publish — UPDATE published_at (как было).
--   * При ошибке Publish — attempts++, last_error/last_attempt_at записаны.
--   * При attempts >= MaxAttempts (default 5 в outbox.Options) — строка
--     перемещается в platform.billing_outbox_dead_letter и удаляется из основной.
--
-- Recovery procedure: docs/runbooks/outbox-dlq-recovery.md.

ALTER TABLE platform.billing_outbox
    ADD COLUMN IF NOT EXISTS attempts        INT         NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error      TEXT,
    ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS platform.billing_outbox_dead_letter (
    id              BIGINT      PRIMARY KEY,            -- оригинальный billing_outbox.id
    aggregate_type  TEXT        NOT NULL,
    aggregate_id    TEXT        NOT NULL,
    event_type      TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    failed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts        INT         NOT NULL,
    last_error      TEXT
);

CREATE INDEX IF NOT EXISTS idx_billing_outbox_dead_letter_failed_at
    ON platform.billing_outbox_dead_letter (failed_at);

-- +goose Down
DROP INDEX IF EXISTS platform.idx_billing_outbox_dead_letter_failed_at;
DROP TABLE IF EXISTS platform.billing_outbox_dead_letter;
ALTER TABLE platform.billing_outbox
    DROP COLUMN IF EXISTS last_attempt_at,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS attempts;

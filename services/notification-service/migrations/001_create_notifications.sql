-- Per-tenant миграция (применяется в схему `tnt_<tenant_id>`).
--
-- Хранит историю уведомлений для аудита и retry-логики.  Сама отправка
-- асинхронна, статусы обновляются sender'ом.

-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    recipient_type  TEXT NOT NULL CHECK (recipient_type IN ('email','sms','push')),
    recipient       TEXT NOT NULL,
    template_id     TEXT NOT NULL,
    vars            JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','failed')),
    attempt_count   INT NOT NULL DEFAULT 0,
    error           TEXT,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_tenant_created
    ON notifications (tenant_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;

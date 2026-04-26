-- +goose Up
CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    recipient   TEXT NOT NULL,
    channel     TEXT NOT NULL CHECK (channel IN ('email','sms','push')),
    template_id TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed')),
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS notifications;

-- +goose Up
CREATE TABLE IF NOT EXISTS client_events (
    id          TEXT PRIMARY KEY,
    client_id   TEXT NOT NULL REFERENCES clients(id),
    tenant_id   TEXT NOT NULL,
    category    TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_client_events_client ON client_events(client_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_client_events_tenant ON client_events(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS client_events;

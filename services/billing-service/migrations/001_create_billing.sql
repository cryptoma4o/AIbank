-- +goose Up
CREATE TABLE IF NOT EXISTS billable_events (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    kind          TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    price_kopecks BIGINT NOT NULL,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_billing_tenant_time ON billable_events(tenant_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_billing_kind ON billable_events(kind);

-- +goose Down
DROP TABLE IF EXISTS billable_events;

-- +goose Up
CREATE TABLE IF NOT EXISTS ubo_nodes (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    app_id      TEXT NOT NULL,
    node_type   TEXT NOT NULL,
    name        TEXT NOT NULL,
    inn         TEXT,
    passport    TEXT,
    stake       DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_ubo      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ubo_nodes_app ON ubo_nodes(app_id, tenant_id);

CREATE TABLE IF NOT EXISTS ubo_edges (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    app_id       TEXT NOT NULL,
    from_node_id TEXT NOT NULL REFERENCES ubo_nodes(id),
    to_node_id   TEXT NOT NULL REFERENCES ubo_nodes(id),
    direct_stake DOUBLE PRECISION NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ubo_edges_app ON ubo_edges(app_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_ubo_edges_to ON ubo_edges(to_node_id);

-- +goose Down
DROP TABLE IF EXISTS ubo_edges;
DROP TABLE IF EXISTS ubo_nodes;

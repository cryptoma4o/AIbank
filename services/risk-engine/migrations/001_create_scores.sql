-- +goose Up
CREATE TABLE IF NOT EXISTS risk_scores (
    id             TEXT PRIMARY KEY,
    application_id TEXT NOT NULL,
    tenant_id      TEXT NOT NULL,
    score          INTEGER NOT NULL,
    blocked        BOOLEAN NOT NULL DEFAULT FALSE,
    flags          TEXT[] NOT NULL DEFAULT '{}',
    fired_rules    TEXT[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_risk_scores_app ON risk_scores(application_id);

-- +goose Down
DROP TABLE IF EXISTS risk_scores;

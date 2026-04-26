-- goose Up

CREATE TABLE IF NOT EXISTS applications (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'draft',
    inn           TEXT NOT NULL,
    ogrn          TEXT NOT NULL,
    workflow_id   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_applications_tenant ON applications(tenant_id);
CREATE INDEX IF NOT EXISTS idx_applications_inn    ON applications(inn);

-- goose Down

DROP TABLE IF EXISTS applications;

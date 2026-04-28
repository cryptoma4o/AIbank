-- +goose Up
-- Per-tenant template (ADR-0002). Применяется к каждой схеме tnt_<id>;
-- search_path выставляется до запуска миграций — никаких префиксов схемы.
CREATE TABLE IF NOT EXISTS documents (
    id               TEXT PRIMARY KEY,
    application_id   TEXT NOT NULL,
    type             TEXT NOT NULL
                         CHECK (type IN (
                             'passport', 'passport_foreign',
                             'charter', 'protocol',
                             'extract', 'agreement'
                         )),
    state            TEXT NOT NULL DEFAULT 'uploaded'
                         CHECK (state IN ('uploaded','parsed','validated','rejected')),
    file_id          TEXT NOT NULL,
    filename         TEXT NOT NULL,
    mime_type        TEXT NOT NULL,
    size_bytes       BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256           TEXT NOT NULL,
    storage_path     TEXT NOT NULL,
    source_type      TEXT NOT NULL
                         CHECK (source_type IN ('client_upload','egrul','esia','generated')),
    source_actor_id  TEXT,
    parsed_data      JSONB,
    uploaded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_documents_application ON documents (application_id);
CREATE INDEX IF NOT EXISTS idx_documents_state       ON documents (state);
CREATE INDEX IF NOT EXISTS idx_documents_uploaded_at ON documents (uploaded_at DESC);

-- +goose Down
DROP TABLE IF EXISTS documents;

-- +goose Up
CREATE TABLE IF NOT EXISTS documents (
    id              TEXT PRIMARY KEY,
    application_id  TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    document_type   TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'uploaded'
                        CHECK (status IN ('uploaded','parsing','parsed','validated','rejected')),
    storage_path    TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type       TEXT NOT NULL,
    checksum        TEXT NOT NULL,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_documents_application ON documents (application_id);
CREATE INDEX idx_documents_tenant ON documents (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS documents;

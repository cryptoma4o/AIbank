-- +goose Up
--
-- Этап 5 формы онбординга — Бенефициарные владельцы (УБО) + FATCA/CRS.
-- Соответствует packages/domain-model/schema.json $defs.UBOGraph (v1.1.0).
-- Граф владения хранится целиком как JSONB (nodes + edges + chains) —
-- запросов "найти всех UBO с долей > 25% в стране X" пока нет, поэтому
-- normalised tables для узлов не делаем.

CREATE TABLE ubo_graphs (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    application_id           TEXT NOT NULL,
    legal_entity_id          TEXT NOT NULL,
    nodes                    JSONB NOT NULL DEFAULT '[]'::JSONB,
    edges                    JSONB NOT NULL DEFAULT '[]'::JSONB,
    ownership_chains         JSONB NOT NULL DEFAULT '[]'::JSONB,
    no_ubo_reason            TEXT,
    eio_as_ubo_confirmation  BOOLEAN NOT NULL DEFAULT FALSE,
    diagram_doc_id           TEXT,
    computed_at              TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один UBO-граф на заявку (этап 5 одноразовый, корректировки — через
-- update existing record).
CREATE UNIQUE INDEX uniq_ubo_graphs_application
    ON ubo_graphs (application_id);

CREATE INDEX idx_ubo_graphs_legal_entity
    ON ubo_graphs (legal_entity_id);

-- +goose Down
DROP TABLE IF EXISTS ubo_graphs;

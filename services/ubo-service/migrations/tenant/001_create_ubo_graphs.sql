-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id> (ADR-0002).
-- НЕ хардкодит имя схемы — db-migrator (ADR-0005) обходит все активные
-- тенанты с этим файлом.

-- Версионируемые snapshot'ы графа владения. На одну (legal_entity_id)
-- допускается N строк, по одной на каждый прогон agent-ubo-tracing.
-- Текущий граф = MAX(version) для legal_entity_id.
CREATE TABLE ubo_graphs (
    id                   TEXT PRIMARY KEY,
    legal_entity_id      TEXT NOT NULL,
    version              INTEGER NOT NULL CHECK (version > 0),
    nodes                JSONB NOT NULL DEFAULT '[]',
    edges                JSONB NOT NULL DEFAULT '[]',
    ubos                 JSONB NOT NULL DEFAULT '[]',
    confidence           DOUBLE PRECISION NOT NULL DEFAULT 0
                            CHECK (confidence >= 0 AND confidence <= 1),
    unresolved_branches  JSONB NOT NULL DEFAULT '[]',
    computed_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    computed_by          TEXT NOT NULL,
    UNIQUE (legal_entity_id, version)
);

-- Быстрый «latest» лукап: ORDER BY version DESC LIMIT 1 идёт по индексу.
CREATE INDEX idx_ubo_graphs_le_ver
    ON ubo_graphs (legal_entity_id, version DESC);

-- Поиск по UBO-узлам внутри JSONB (например, «все графы, где per_42 — UBO»).
CREATE INDEX idx_ubo_graphs_ubos_gin
    ON ubo_graphs USING GIN (ubos jsonb_path_ops);

-- +goose Down
DROP TABLE IF EXISTS ubo_graphs;

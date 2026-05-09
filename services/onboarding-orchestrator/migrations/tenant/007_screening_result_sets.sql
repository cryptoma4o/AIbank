-- +goose Up
--
-- Этап 7 формы онбординга — сводный набор AML-проверок по заявке.
-- Соответствует packages/domain-model/schema.json $defs.ScreeningResultSet
-- (v1.1.0). Sanctions / PEP / adverse_media / anti_fraud — JSONB,
-- т.к. структуры heterogenous и query'ы пока нет.
--
-- Один screening на заявку (актуальное состояние; история — в audit-log).

CREATE TABLE screening_result_sets (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    application_id           TEXT NOT NULL,
    sanctions_results        JSONB NOT NULL DEFAULT '[]'::JSONB,
    pep_results              JSONB NOT NULL DEFAULT '[]'::JSONB,
    adverse_media_hits       JSONB NOT NULL DEFAULT '[]'::JSONB,
    okved_consistency_score  NUMERIC,
    turnover_realism_score   NUMERIC,
    anti_fraud_signals       JSONB,
    performed_at             TIMESTAMPTZ NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (okved_consistency_score IS NULL OR (okved_consistency_score >= 0 AND okved_consistency_score <= 100)),
    CHECK (turnover_realism_score IS NULL OR (turnover_realism_score >= 0 AND turnover_realism_score <= 100))
);

CREATE UNIQUE INDEX uniq_screening_result_sets_application
    ON screening_result_sets (application_id);

-- +goose Down
DROP TABLE IF EXISTS screening_result_sets;

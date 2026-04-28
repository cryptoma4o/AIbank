-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id>.
-- Применяется через db-migrator (ADR-0005), не сервисом risk-engine.
--
-- ВАЖНО: миграция использует current_schema() и НЕ хардкодит имя схемы.
-- Тем самым один и тот же файл применяется к любой схеме тенанта.
--
-- Структура таблицы соответствует docs/domain-model.md § 2.7 RiskAssessment.
-- JSONB-колонки используются для вложенных value objects (model, factors,
-- rules_triggered, screening_results, explanation), чтобы избежать раздувания
-- набора таблиц на этапе MVP — нормализация добавится по мере роста запросов.

CREATE TABLE risk_assessments (
    id                TEXT PRIMARY KEY,
    application_id    TEXT NOT NULL,
    legal_entity_id   TEXT,
    score             NUMERIC(6, 4) NOT NULL CHECK (score >= 0 AND score <= 1),
    category          TEXT NOT NULL CHECK (category IN ('LOW', 'MEDIUM', 'HIGH')),

    model             JSONB NOT NULL DEFAULT '{}',
    factors           JSONB NOT NULL DEFAULT '[]',
    rules_triggered   JSONB NOT NULL DEFAULT '[]',
    screening_results JSONB NOT NULL DEFAULT '{}',
    explanation       JSONB NOT NULL DEFAULT '{}',
    recommendation    TEXT NOT NULL CHECK (
        recommendation IN ('AUTO_APPROVE', 'MANUAL_REVIEW', 'DECLINE_RECOMMENDED')
    ),

    reviewed_by       TEXT,
    review_decision   TEXT,
    review_comment    TEXT,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_risk_assessments_app
    ON risk_assessments (application_id, created_at DESC);

CREATE INDEX idx_risk_assessments_recommendation
    ON risk_assessments (recommendation, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS risk_assessments;

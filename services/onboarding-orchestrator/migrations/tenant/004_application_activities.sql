-- +goose Up
--
-- Этап 3 формы онбординга — AML-сведения о деятельности заявителя.
-- Соответствует packages/domain-model/schema.json $defs.ApplicationActivity
-- (v1.1.0). Контрагенты / операционная модель / источник средств — JSONB,
-- т.к. они структурные, но не используются в WHERE-запросах.

CREATE TABLE application_activities (
    id                    TEXT PRIMARY KEY,
    tenant_id             TEXT NOT NULL,
    application_id        TEXT NOT NULL,
    business_description  TEXT NOT NULL,
    business_category     TEXT NOT NULL CHECK (business_category IN ('low_risk','medium_risk','high_risk')),
    top_suppliers         JSONB NOT NULL DEFAULT '[]'::JSONB,
    top_buyers            JSONB NOT NULL DEFAULT '[]'::JSONB,
    operational_model     JSONB,
    funds_source          JSONB NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uniq_application_activities_application
    ON application_activities (application_id);

-- +goose Down
DROP TABLE IF EXISTS application_activities;

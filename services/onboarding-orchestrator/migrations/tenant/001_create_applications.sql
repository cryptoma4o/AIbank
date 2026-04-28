-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id>.
-- Имя схемы НЕ хардкодится — миграция полагается на search_path,
-- выставленный db-migrator (ADR-0005).  Один и тот же файл применяется
-- ко всем тенантам.
--
-- Источник истины модели: docs/domain-model.md § 2.2 (Application).
-- Состояния перечислены явно через CHECK (см. internal/domain/state.go).
-- FK на applicants — soft (другая схема + другой сервис identity-service);
-- проверка делается на уровне приложения, а не БД.

CREATE TABLE applications (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    applicant_id        TEXT NOT NULL,
    legal_entity_id     TEXT,
    legal_entity_type   TEXT NOT NULL CHECK (legal_entity_type IN ('IP','LLC','JSC','NPF')),
    channel             TEXT NOT NULL CHECK (channel IN ('web','mobile','courier','branch')),
    state               TEXT NOT NULL CHECK (state IN (
                            'draft','identifying','collecting_documents','validating',
                            'waiting_for_client','risk_assessing',
                            'auto_approved','manual_review','approved','approved_with_edd',
                            'requires_more_info','opening_account',
                            'account_opened','declined','abandoned'
                        )),
    product_codes       TEXT[] NOT NULL DEFAULT '{}',
    workflow_id         TEXT NOT NULL,

    risk_assessment_id  TEXT,
    decision_id         TEXT,
    account_ids         TEXT[] NOT NULL DEFAULT '{}',

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    archived_at         TIMESTAMPTZ
);

CREATE INDEX idx_applications_state       ON applications (state);
CREATE INDEX idx_applications_applicant   ON applications (applicant_id);
CREATE INDEX idx_applications_workflow    ON applications (workflow_id);
CREATE INDEX idx_applications_created_at  ON applications (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS applications;

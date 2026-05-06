-- +goose Up
--
-- Этап 4 формы онбординга — ЕИО и представители юрлица. На одну заявку
-- может быть несколько представителей; ровно один из них обязан быть
-- primary (основной ЕИО). Соответствует $defs.Representative + расширенным
-- полям Person из packages/domain-model/schema.json (v1.1.0).
--
-- Персональные данные хранятся вместе с Representative в orchestrator
-- (а не в identity-service), потому что это атрибуты заявки, а не
-- учётной записи пользователя.

CREATE TABLE representatives (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    application_id           TEXT NOT NULL,
    legal_entity_id          TEXT NOT NULL,
    last_name                TEXT NOT NULL,
    first_name               TEXT NOT NULL,
    middle_name              TEXT,
    birth_date               DATE NOT NULL,
    birth_place              TEXT,
    citizenship              TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    inn                      TEXT,
    snils                    TEXT,
    id_document              JSONB NOT NULL,
    registration_address     JSONB,
    actual_address           JSONB,
    foreigner_info           JSONB,
    authority                JSONB NOT NULL,
    pdl_declaration          JSONB,
    is_primary               BOOLEAN NOT NULL DEFAULT FALSE,
    is_signatory             BOOLEAN NOT NULL DEFAULT FALSE,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один primary-представитель на заявку.
CREATE UNIQUE INDEX uniq_representatives_primary_per_application
    ON representatives (application_id) WHERE is_primary;

CREATE INDEX idx_representatives_application
    ON representatives (application_id);

-- +goose Down
DROP TABLE IF EXISTS representatives;

-- +goose Up
--
-- Этап 2 формы онбординга — расширенная анкета юрлица. Соответствует
-- packages/domain-model/schema.json $defs.LegalEntityProfile (v1.1.0).
-- Структурированные адреса, контакты, лицензии и СРО сохраняем как
-- JSONB — для MVP этого достаточно, нормализованные таблицы добавим
-- если появятся сценарии поиска по полям внутри них.

CREATE TABLE legal_entity_profiles (
    id                          TEXT PRIMARY KEY,
    tenant_id                   TEXT NOT NULL,
    application_id              TEXT NOT NULL,
    legal_entity_id             TEXT NOT NULL,
    opf_code                    TEXT,
    registration_authority      TEXT,
    authorized_capital_amount   NUMERIC,
    authorized_capital_currency TEXT,
    legal_address_struct        JSONB,
    actual_address_struct       JSONB,
    actual_same_as_legal        BOOLEAN NOT NULL DEFAULT FALSE,
    postal_address_struct       JSONB,
    postal_same_as_legal        BOOLEAN NOT NULL DEFAULT FALSE,
    okved_main_v2               TEXT,
    okved_additional_v2         TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    licenses                    JSONB NOT NULL DEFAULT '[]'::JSONB,
    sro_membership              JSONB NOT NULL DEFAULT '[]'::JSONB,
    contacts                    JSONB,
    employees_count             INTEGER,
    revenue_last_year_amount    NUMERIC,
    revenue_last_year_currency  TEXT,
    tax_regime                  TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один профиль на заявку — это ключевой бизнес-инвариант: анкета
-- редактируется до отправки документов, сохраняется один раз.
CREATE UNIQUE INDEX uniq_legal_entity_profiles_application
    ON legal_entity_profiles (application_id);

-- Lookup по legal_entity_id для админки.
CREATE INDEX idx_legal_entity_profiles_legal_entity
    ON legal_entity_profiles (legal_entity_id);

-- +goose Down
DROP TABLE IF EXISTS legal_entity_profiles;

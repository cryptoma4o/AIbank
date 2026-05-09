-- +goose Up
--
-- Этап 9 формы онбординга — открытие счёта.
-- Соответствует packages/domain-model/schema.json $defs.Account и
-- AccountAgreements (v1.1.0).
--
-- Эта таблица — orchestrator-side представление счёта (что банк решил
-- открыть). Реальное создание в АБС банка делает abs-connector;
-- abs_reference = ID в АБС после успеха.

CREATE TABLE accounts (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    application_id           TEXT NOT NULL,
    legal_entity_id          TEXT NOT NULL,
    account_number           TEXT,
    bik                      TEXT,
    bank_name                TEXT,
    currency                 TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    account_type             TEXT NOT NULL CHECK (account_type IN (
        'settlement', 'deposit', 'loan',
        'special', 'foreign_currency', 'escrow', 'nominal'
    )),
    correspondent_account    TEXT,
    tariff_plan              TEXT,
    agreements               JSONB NOT NULL,
    monitoring_profile_id    TEXT,
    abs_reference            TEXT,
    opened_at                TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Один счёт на заявку для MVP (multi-account per application — отдельная
-- задача, потребует промежуточной сущности AccountSet).
CREATE UNIQUE INDEX uniq_accounts_application
    ON accounts (application_id);

CREATE INDEX idx_accounts_legal_entity
    ON accounts (legal_entity_id);

-- +goose Down
DROP TABLE IF EXISTS accounts;

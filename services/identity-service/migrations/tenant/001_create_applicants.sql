-- +goose Up
--
-- ШАБЛОН миграции, применяемой к каждой схеме tnt_<id>.
-- Применяется через db-migrator (ADR-0005); имя схемы НЕ хардкодится — миграция
-- запускается с уже выставленным search_path.

-- Заявители (клиенты банка) — основная сущность онбординга.
-- ИНН физлица — 12 цифр. Телефон храним строкой (может содержать +7..., форматирование
-- — ответственность приложения; в БД нормализованный вид).
--
-- Шифрование PII (см. docs/security-architecture.md § 5.2 + ADR-0002):
--   * BYTEA-колонки *_enc хранят Vault Transit ciphertext ("vault:v1:<base64>")
--     для поля per-tenant ключом aibank-tenant-<tenant_id>.
--   * Старые TEXT-колонки (inn, passport_series_text, snils_text) помечены
--     как DEPRECATED и оставлены для обратной совместимости — на этапе
--     раскатки приложение пишет в обе колонки, читает из BYTEA при наличии
--     Transit-клиента, иначе из TEXT. Удаление plaintext-колонок —
--     follow-up миграцией после batch-rewrap'а legacy-строк.
--   * Полное имя (full_name) и телефон (phone) НЕ шифруются — по ним идёт
--     поиск/индексация. Защита — TDE + access controls.
CREATE TABLE applicants (
    id             TEXT PRIMARY KEY,
    inn            CHAR(12) NOT NULL UNIQUE,         -- DEPRECATED (plaintext): мигрировать в inn_enc
    phone          TEXT NOT NULL,
    full_name      TEXT NOT NULL,
    esia_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    esia_subject   TEXT NULL,

    -- Field-level encrypted (Vault Transit, AES-256-GCM96).
    -- NULL допустимы: у нерезидентов / ИП может не быть СНИЛС, паспортные
    -- данные приходят отдельным шагом онбординга.
    passport_series_enc BYTEA NULL,
    passport_number_enc BYTEA NULL,
    snils_enc           BYTEA NULL,
    inn_enc             BYTEA NULL,                   -- дубликат inn в шифрованном виде (см. follow-up миграцию)

    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_applicants_phone ON applicants (phone);
CREATE INDEX idx_applicants_esia  ON applicants (esia_subject) WHERE esia_subject IS NOT NULL;

-- Индексы по *_enc колонкам не создаём: ciphertext per row уникален из-за
-- random nonce внутри Vault Transit, поэтому B-tree бесполезен. Поиск по
-- зашифрованным полям — отдельный TODO (deterministic encryption / blind index).

-- +goose Down
DROP TABLE IF EXISTS applicants;

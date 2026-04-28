-- +goose Up
--
-- Согласия субъекта ПДн (152-ФЗ ст. 9). Применяется к каждой схеме tnt_<id>;
-- имя схемы НЕ хардкодится — db-migrator (ADR-0005) запускает миграцию с уже
-- выставленным search_path.
--
-- Семантика хранения — append-only:
--   - INSERT при первоначальной выдаче или повторной выдаче после отзыва.
--   - Отзыв (Revoke) реализуется как UPDATE revoked_at = NOW() ТОЛЬКО для
--     текущей активной строки. Триггер запрещает любые иные UPDATE
--     (включая модификацию granted/version/applicant_id) и любые DELETE.
--
-- Уникальность активного согласия каждого типа на одного заявителя
-- обеспечивается частичным уникальным индексом по (applicant_id, consent_type)
-- WHERE revoked_at IS NULL.

CREATE TABLE consents (
    id              TEXT PRIMARY KEY,
    applicant_id    TEXT NOT NULL REFERENCES applicants(id) ON DELETE RESTRICT,
    consent_type    TEXT NOT NULL
                        CHECK (consent_type IN ('data_processing','marketing','biometrics')),
    granted         BOOLEAN NOT NULL,
    version         TEXT NOT NULL,
    ip_address      TEXT NULL,
    user_agent      TEXT NULL,
    signature       TEXT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at      TIMESTAMPTZ NULL
);

-- Один активный consent каждого типа на заявителя. Отозванные строки в
-- индекс не входят — возможно повторное предоставление согласия после отзыва.
CREATE UNIQUE INDEX uq_consents_active
    ON consents (applicant_id, consent_type)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_consents_applicant ON consents (applicant_id);
CREATE INDEX idx_consents_recorded  ON consents (recorded_at DESC);

-- Append-only enforcement.  Разрешён ровно один тип UPDATE: проставление
-- revoked_at (NULL → not-NULL) у активной строки. Любые прочие изменения
-- любых полей и DELETE — запрещены.
CREATE OR REPLACE FUNCTION enforce_consents_append_only()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'consents append-only: DELETE запрещён (152-ФЗ журнал согласий)';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        -- Допустимо только проставление revoked_at у активной записи.
        IF OLD.revoked_at IS NOT NULL THEN
            RAISE EXCEPTION 'consents append-only: запись уже отозвана, повторный UPDATE запрещён';
        END IF;
        IF NEW.revoked_at IS NULL THEN
            RAISE EXCEPTION 'consents append-only: разрешён только UPDATE revoked_at NULL→not-NULL';
        END IF;
        IF NEW.id            <> OLD.id
           OR NEW.applicant_id <> OLD.applicant_id
           OR NEW.consent_type <> OLD.consent_type
           OR NEW.granted      IS DISTINCT FROM OLD.granted
           OR NEW.version      <> OLD.version
           OR NEW.ip_address   IS DISTINCT FROM OLD.ip_address
           OR NEW.user_agent   IS DISTINCT FROM OLD.user_agent
           OR NEW.signature    IS DISTINCT FROM OLD.signature
           OR NEW.recorded_at  <> OLD.recorded_at THEN
            RAISE EXCEPTION 'consents append-only: разрешено изменение только поля revoked_at';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER no_delete BEFORE DELETE ON consents
    FOR EACH ROW EXECUTE FUNCTION enforce_consents_append_only();

CREATE TRIGGER no_update BEFORE UPDATE ON consents
    FOR EACH ROW EXECUTE FUNCTION enforce_consents_append_only();

-- +goose Down
DROP TABLE IF EXISTS consents;
DROP FUNCTION IF EXISTS enforce_consents_append_only();

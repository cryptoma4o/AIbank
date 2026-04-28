-- +goose Up
-- 152-ФЗ ст. 14 "право на забвение".
-- Когда клиент запрашивает удаление, PII-поля скрабятся (full_name/phone/inn
-- очищаются, passport_*/snils → NULL), но applicant-row сохраняется ради
-- FK-целостности audit-логов (5 лет по 115-ФЗ).

ALTER TABLE applicants
    ADD COLUMN IF NOT EXISTS forgotten_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS forgotten_by TEXT NULL;

-- Индекс для отчётов "сколько applicant'ов попросили забыть за период".
CREATE INDEX IF NOT EXISTS idx_applicants_forgotten_at
    ON applicants (forgotten_at)
    WHERE forgotten_at IS NOT NULL;

COMMENT ON COLUMN applicants.forgotten_at
    IS '152-ФЗ ст. 14: timestamp когда applicant запросил удаление PII';
COMMENT ON COLUMN applicants.forgotten_by
    IS '152-ФЗ ст. 14: actor_id (user_id операцииста или service_id), выполнивший scrub';

-- +goose Down
ALTER TABLE applicants
    DROP COLUMN IF EXISTS forgotten_by,
    DROP COLUMN IF EXISTS forgotten_at;

-- +goose Up
--
-- Бизнес-правило (выбрано 2026-05-06): один applicant_id может иметь
-- максимум одну заявку в рамках тенанта за всю историю. Strict вариант
-- для compliance — повторное создание блокируется даже после declined /
-- abandoned / account_opened. Если в будущем перейдём на active-only,
-- достаточно заменить этот индекс на partial с
--   ... WHERE state NOT IN ('account_opened','declined','abandoned')
-- без миграции данных.

CREATE UNIQUE INDEX uniq_applications_tenant_applicant
    ON applications (tenant_id, applicant_id);

-- Старый non-unique idx_applications_applicant из 001_create_applications.sql
-- оставляем как есть — он обслуживает запросы по applicant без tenant_id.

-- +goose Down
DROP INDEX IF EXISTS uniq_applications_tenant_applicant;

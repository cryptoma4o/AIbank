-- +goose Up
--
-- Разрешаем role=applicant в platform.users чтобы applicant'ы могли
-- логиниться через тот же /v1/auth/login flow (минимальный путь #2 для
-- pre-MVP staging). Telephone/SMS-based auth — отдельная задача.
--
-- tenant_role_consistency CHECK уже допускает любую роль ≠ platform.admin
-- с обязательным tenant_id, поэтому applicant автоматически получает
-- requirement tenant_id IS NOT NULL — корректно для multi-tenant модели.

ALTER TABLE platform.users DROP CONSTRAINT users_role_check;

ALTER TABLE platform.users ADD CONSTRAINT users_role_check
    CHECK (role IN (
        'platform.admin',
        'bank.operator',
        'bank.compliance_officer',
        'bank.admin',
        'applicant'
    ));

-- +goose Down
ALTER TABLE platform.users DROP CONSTRAINT users_role_check;

ALTER TABLE platform.users ADD CONSTRAINT users_role_check
    CHECK (role IN (
        'platform.admin',
        'bank.operator',
        'bank.compliance_officer',
        'bank.admin'
    ));

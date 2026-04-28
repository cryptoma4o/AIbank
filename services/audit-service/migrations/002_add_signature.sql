-- +goose Up
-- Опциональные поля для криптографической подписи audit-event'а.
-- ADR-0010 + docs/pilot-readiness.md § 1.4 (Audit log с криптографической подписью).
--
-- Сейчас (Pre-MVP) hash chain (SHA-256 prev_hash) даёт tamper-evident
-- свойство, но не non-repudiation: любой с доступом к БД может пересчитать
-- цепочку с подменённым payload'ом. Криптоподпись каждого row даёт настоящую
-- невозможность отказа от события.
--
-- Поля nullable — backwards compat:
--   * existing rows без подписи остаются валидными;
--   * audit-verifier проверяет signature ТОЛЬКО если она присутствует
--     (см. tools/audit-verifier/internal/verifier/verifier.go::VerifyChain).
--
-- Алгоритмы:
--   * "ed25519" — Pre-MVP, доступен встроенно (crypto/ed25519). Использовать
--     до получения СКЗИ-лицензий.
--   * "gost-2012-256" / "gost-2012-512" — после интеграции КриптоПро/VipNet
--     (см. packages/signature/, lead-time 2-4 нед лицензии).
--
-- signer_key_id — opaque строка, формат зависит от провайдера:
--   * для ed25519: hex-encoded SHA-256 fingerprint публичного ключа;
--   * для ГОСТ: thumbprint сертификата из СКЗИ.

ALTER TABLE audit.events
    ADD COLUMN IF NOT EXISTS signature           BYTEA,
    ADD COLUMN IF NOT EXISTS signature_algorithm TEXT,
    ADD COLUMN IF NOT EXISTS signer_key_id       TEXT;

-- +goose Down
ALTER TABLE audit.events
    DROP COLUMN IF EXISTS signer_key_id,
    DROP COLUMN IF EXISTS signature_algorithm,
    DROP COLUMN IF EXISTS signature;

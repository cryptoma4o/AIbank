# packages/signature

Провайдер-абстракция УКЭП (Усиленной Квалифицированной Электронной Подписи) по
ГОСТ Р 34.10-2012 и хеша Streebog (Р 34.11-2012). Используется в
`document-service`, `identity-service`, `audit-service` для подписания
документов клиентом, аутентификации через УКЭП и подписания audit-событий
соответственно.

**Статус**: Pre-MVP. Реальные провайдеры (КриптоПро / VipNet) — отдельные
пакеты, которые подключатся после получения СКЗИ-лицензий банка-партнёра
(см. `docs/pilot-readiness.md` § 1.4, lead-time 2-4 недели).

Сейчас в репо только `MockSignatureProvider` для unit/integration-тестов.

## Использование

```go
import "github.com/aibank/platform/packages/signature"

// Production setup (когда лицензии получены):
//   provider, err := cryptopro.NewProvider(cryptopro.Config{...})
// 
// Pre-MVP / тесты:
provider := signature.NewMockSignatureProvider()

// Подписание
signed, err := provider.Sign(ctx, payload, "client-cert-id")
// signed.Signature — DER-encoded ГОСТ-подпись (mock: SHA-256 хеш)
// signed.PayloadHash — hex-encoded хеш для re-verification
// signed.SignedAt — UTC timestamp

// Верификация
result, _ := provider.Verify(ctx, signed)
if !result.Valid {
    log.Warn("invalid signature", "reason", result.ErrorMessage)
}
```

## Контракт провайдера

| Метод | Назначение |
|-------|-----------|
| `Sign(payload, certID) → SignedPayload` | Подписать payload через указанный сертификат СКЗИ |
| `Verify(signed) → VerificationResult` | Проверить валидность подписи + срока сертификата + (опционально) chain |
| `ListCertificates() → []Certificate` | Список доступных сертификатов (для UI выбора клиентом) |
| `GetCertificate(certID) → Certificate` | Полная информация о сертификате (для chain validation) |
| `Name() → string` | Имя реализации ("cryptopro" / "vipnet" / "mock") |

## Структура SignedPayload

Хранится в БД как JSONB или передаётся между сервисами. Содержит достаточно
информации для верификации **без** доступа к исходному payload (только хеш +
signature + cert).

| Поле | Описание |
|------|----------|
| `signature` | DER-encoded ГОСТ-подпись |
| `payload_hash` | hex Streebog-256/512 (или SHA-256 в Mock) |
| `hash_algorithm` | "streebog256" / "streebog512" / "sha256" |
| `signer_certificate` | PEM/DER X.509 (hex-encoded в JSON) |
| `certificate_chain` | опционально: до корневого УЦ |
| `signed_at` | UTC ISO 8601 |
| `provider` | "cryptopro" / "vipnet" / "mock" |
| `provider_version` | версия СКЗИ (audit trail) |
| `container` | опциональный CMS/CAdES |

## Хранение сертификатов

Приватные ключи **никогда** не возвращаются из СКЗИ — они остаются внутри
криптомодуля. Для production:

- Vault path: `secret/data/aibank/<service>/signature/certs/<tenant_id>/`
- Содержимое: `certificate` (PEM) + ссылка на ключ в СКЗИ через `cert_id`
- Никогда не в Git, никогда в env-переменных prod

См. `packages/secrets/factory.go` — provider-чейн для чтения из Vault.

## Реальные провайдеры (план)

| Пакет | Когда | Зависимости |
|-------|-------|-------------|
| `signature/cryptopro` | После получения лицензий КриптоПро | cgo bindings к CryptoAPI 2.0 |
| `signature/vipnet` | По запросу банка-партнёра | VipNet CSP API |
| `signature/mock` | Сейчас | — |

Все реальные провайдеры реализуют `SignatureProvider` — переключение через
factory + Vault config, без изменений в потребителях.

## Связанные ADR

- ADR-0010 — billing & audit log (signed audit-events)
- `docs/security-architecture.md` § 6.3 — УКЭП в архитектуре безопасности
- `docs/compliance-map.md` — 63-ФЗ требования к УКЭП

## Tests

```bash
cd packages/signature
go test ./... -count=1
```

Mock покрыт unit-тестами: happy-path, deterministic Sign, expired cert,
verify chain, cert not found, list/get certificates.

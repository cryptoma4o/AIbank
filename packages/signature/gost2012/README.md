# `packages/signature/gost2012` — STUB-подписант

> **DO NOT USE IN PRODUCTION.**
>
> Этот пакет — **placeholder**, не реальная реализация ГОСТ Р 34.10-2012 /
> Streebog. Внутри используется keyed-SHA-256. Размер выходной "подписи"
> (32 байта) совпадает с ГОСТ-2012-256 только для visual-identification
> и для упрощения swap'а на реальный КриптоПро.

## Зачем нужен этот пакет

1. **On-prem dev/staging банков**, у которых ed25519 не принимается
   комплаенсом "по визуальной идентичности с ГОСТ-2012". Stub позволяет
   audit-service писать события с маркером `gost-2012-256-stub` и
   репетировать flow до получения СКЗИ-лицензий.
2. **Подготовка к swap'у на реальный СКЗИ** (КриптоПро / VipNet),
   lead-time лицензирования 2-4 недели через банк.
3. **Громкий маркер**: алгоритм `gost-2012-256-stub` намеренно отличается
   от целевого `gost-2012-256` — verifier при production rollout сразу
   увидит mismatch и отклонит stub-payload'ы. Это feature, не баг.

## API (зеркалит `packages/signature/ed25519`)

| Функция / метод                      | Что делает                                           |
| ------------------------------------ | ---------------------------------------------------- |
| `GenerateStub() (*StubSigner, err)`  | Новая stub-keypair через `crypto/rand` (для dev).    |
| `FromStubKey(priv []byte)`           | Из 32-байтного raw private key (детерминированно).   |
| `FromBase64Stub(b64 string)`         | Convenience-обёртка для ENV / Vault.                 |
| `FromEnv(envName string)`            | Из env-переменной (пусто → `(nil, nil)`).            |
| `(*StubSigner) Sign(digest)`         | `SHA-256(priv \|\| digest)`, 32 байта.               |
| `(*StubSigner) Verify(digest, sig)`  | Recompute + compare (не constant-time).              |
| `(*StubSigner) KeyID()`              | `"STUB-" + hex(sha256(pub))[:32]` — для SOC grep.    |
| `(*StubSigner) PublicKey()`          | 32 байта — derived `sha256(priv)`.                   |
| `(*StubSigner) PublicKeyBase64()`    | Base64 от PublicKey.                                 |
| `KeyIDFromPublicKey(pub)`            | Verifier-side вычисление KeyID.                      |
| `PublicKeyFromBase64(b64)`           | Распаковка из base64.                                |
| `Algorithm` (const)                  | `"gost-2012-256-stub"`                               |
| `KeySize`, `SignatureSize` (const)   | `32` (visual-match с ГОСТ-2012-256).                 |

### Узкий audit-EventSigner интерфейс

```go
type EventSigner interface {
    Sign(digest []byte) ([]byte, error)
    KeyID() string
}
```

`*StubSigner` реализует этот интерфейс (как и `*ed25519.Signer`). Audit-service
дописывает алгоритм `Algorithm = "gost-2012-256-stub"` рядом с подписью —
verifier по нему выбирает правильный verify-провайдер.

## Свойства подписи

- **Длина**: 32 байта (== ГОСТ-2012-256 / Streebog-256).
- **Детерминизм**: `Sign(digest)` идемпотентен (как ed25519, но по другой причине).
- **Криптостойкость**: НЕТ. Это keyed-SHA-256, не настоящий ГОСТ-подпись.
- **KeyID**: всегда префиксован `STUB-` — видно в логах, в БД, в JSON.

## Путь миграции на реальный КриптоПро

Когда банк предоставит лицензию:

1. Создать пакет `packages/signature/cryptopro` с реальной реализацией
   (cgo-bindings к КриптоПро CSP или внешний go-gost-crypto lib).
2. В consumer-сервисах (`audit-service`, `document-service`) заменить
   импорт `packages/signature/gost2012` → `packages/signature/cryptopro`.
3. Поменять `Algorithm`-константу с `"gost-2012-256-stub"` на
   `"gost-2012-256"`.
4. Verifier обновляется так, что **отказывается** проверять подписи с
   алгоритмом `gost-2012-256-stub` в production-окружении (feature flag
   по `ENV` / per-tenant config).
5. Удалить `packages/signature/gost2012/` — или оставить под build-tag
   `dev` для off-prod тестов.

## Что этот пакет НЕ делает

- ❌ Не реализует ГОСТ Р 34.10-2012 (signature).
- ❌ Не реализует Р 34.11-2012 / Streebog (hash).
- ❌ Не сертифицирован ФСБ.
- ❌ Не подходит для квалифицированной электронной подписи.
- ❌ Не интегрирован в audit-service `main.go` или handler'ы — только
   пакет. Подключение — отдельным PR после Security review.

## Связанные документы

- `docs/security-architecture.md` § 6.3 — требования УКЭП по ГОСТ-2012.
- `docs/compliance-map.md` — соответствие ФСБ-сертификации.
- `packages/signature/ed25519/` — реально работающий compact signer
  для audit-events до получения СКЗИ-лицензий.
- `packages/signature/provider.go` — абстрактный SignatureProvider
  для документ-уровня УКЭП (отдельный flow от audit-events).

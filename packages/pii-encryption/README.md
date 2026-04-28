# pii-encryption

Высокоуровневая обёртка над [`packages/secrets`](../secrets) для field-level
шифрования PII (паспорт, СНИЛС, ИНН физлица, телефон, расчётный счёт).

Соответствует требованиям [`docs/security-architecture.md` § 5.2](../../docs/security-architecture.md).

## Зачем отдельный пакет

`packages/secrets/transit.go` даёт низкоуровневый `Transit` интерфейс
(`Encrypt(keyName, plaintext)`). Пакет `pii-encryption` добавляет:

1. **Per-field key isolation** — каждое логическое поле шифруется своим
   Vault Transit-ключом (`pii-passport-number`, `pii-snils`, ...). При
   компрометации одного ключа blast radius ограничен соответствующей
   колонкой; ротация независимая (см. § 6.2 ADR ключевой иерархии).
2. **Versioned ciphertext** — формат `vault:v<N>:<base64>` нативно
   поддерживает Vault Transit. После `vault write -f transit/keys/<k>/rotate`
   старые ciphertext'ы остаются readable, новые шифруются текущей версией.
3. **Searchable hash** — `EncryptHash(field, plaintext)` возвращает
   детерминированный HMAC-SHA256 для equals-lookup'ов (`WHERE snils_hash =
   $1`). НЕ обратимо — это hash, не deterministic encryption.
4. **Whitelist полей** — `FieldType` enum, неизвестные поля отвергаются.
   Юрлицо ИНН (10 цифр, публичный идентификатор по ЕГРЮЛ) исключён.

## Поддерживаемые поля

| `FieldType`           | Vault Transit key       | Описание                                     |
|-----------------------|-------------------------|----------------------------------------------|
| `FieldPassportNumber` | `pii-passport-number`   | Серия+номер паспорта РФ                      |
| `FieldPassportIssuer` | `pii-passport-issuer`   | Кем выдан                                    |
| `FieldSNILS`          | `pii-snils`             | 11 цифр СНИЛС                                |
| `FieldPersonalINN`    | `pii-personal-inn`      | ИНН физлица (12 цифр)                        |
| `FieldBankAccount`    | `pii-bank-account`      | Расчётный счёт клиента (152-ФЗ)              |
| `FieldPhone`          | `pii-phone`             | Телефон (для регуляторных кейсов)            |

**Важно:** ИНН юрлица (10 цифр) НЕ шифруется — это публичный идентификатор
по ЕГРЮЛ.

## Usage

```go
import (
    piiencryption "github.com/aibank/platform/packages/pii-encryption"
    "github.com/aibank/platform/packages/secrets"
)

// 1. Wiring на старте сервиса.
transit, err := secrets.NewTransitClient(secrets.TransitConfig{
    Address: os.Getenv("VAULT_ADDR"),
    Token:   os.Getenv("VAULT_TOKEN"),
})
if err != nil {
    return err
}

// HashSecret поднимаем из Vault KV (>= 32 байта). НЕ хранить в env vars.
hashSecret, err := provider.GetSecret(ctx, "pii-encryption/hash-secret")
if err != nil {
    return err
}

enc, err := piiencryption.NewVaultPIIEncryptor(piiencryption.VaultPIIConfig{
    Transit:    transit,
    HashSecret: []byte(hashSecret),
})
if err != nil {
    return err
}

// На bootstrap'е идемпотентно создаём все Transit-ключи.
if err := enc.EnsureKeys(ctx); err != nil {
    return err
}

// 2. Шифрование при сохранении applicant'а.
encryptedSNILS, err := enc.Encrypt(ctx, piiencryption.FieldSNILS, applicant.SNILS)
if err != nil {
    return err
}
snilsHash, err := enc.EncryptHash(ctx, piiencryption.FieldSNILS, applicant.SNILS)
if err != nil {
    return err
}
// INSERT INTO applicants (snils_ciphertext, snils_hash, ...) VALUES ($1, $2, ...)

// 3. Поиск по СНИЛС (equals).
hash, _ := enc.EncryptHash(ctx, piiencryption.FieldSNILS, requestSNILS)
// SELECT * FROM applicants WHERE snils_hash = $1

// 4. Чтение и расшифровка.
plain, err := enc.Decrypt(ctx, piiencryption.FieldSNILS, row.SNILSCiphertext)
```

## Тесты

В тестах используйте `MockPIIEncryptor` — детерминистический XOR без сети:

```go
enc := piiencryption.NewMockPIIEncryptor()
ct, _ := enc.Encrypt(ctx, piiencryption.FieldSNILS, "123-456-789 01")
// ct == "vault:v1:mock-..." — стабильный round-trip
```

Mock покрывает:

- `Encrypt` / `Decrypt` round-trip;
- детерминизм (одинаковый plaintext → одинаковый ciphertext);
- per-field изоляцию (разные ciphertext'ы под разными `FieldType`);
- whitelist (`legal-inn`, `unknown` → ошибка).

`MockPIIEncryptor` НЕ криптография — только для тестов и dev-сценариев.

## Безопасность

- **Никаких plaintext'ов в логах.** Ошибки оборачивают только `FieldType`,
  не значение.
- **Empty plaintext проходит насквозь** — `""` → `""` без обращения к
  Vault и без расхода на HMAC.
- **HashSecret >= 32 байта** — проверяется в конструкторе. Достаётся из
  Vault KV, не из env vars.
- **Defensive copy** `HashSecret` в конструкторе — caller'у безопасно
  занулять slice.
- **Версионирование hash'ей** — префикс `h1:` зарезервирован для будущего
  upgrade'а алгоритма (`h2:` → SHA-3, например).

## Ссылки

- [`docs/security-architecture.md` § 5.2 — Field-level encryption](../../docs/security-architecture.md)
- [`packages/secrets/README.md`](../secrets/README.md) — низкоуровневый
  Vault wrapper.

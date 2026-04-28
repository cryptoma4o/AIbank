// Package piiencryption provides a high-level wrapper around Vault Transit
// for field-level encryption of PII fields (паспорт, СНИЛС, ИНН физлица,
// телефон, расчётный счёт). См. docs/security-architecture.md § 5.2.
//
// Архитектура:
//
//   - Per-field key isolation: каждое логическое поле шифруется своим Vault
//     Transit-ключом ("pii-passport-number", "pii-snils", ...). Это
//     уменьшает blast radius при компрометации отдельного ключа и упрощает
//     ротацию: ротировать паспортный ключ можно независимо от СНИЛС.
//   - Versioned ciphertext: ciphertext возвращается в native-Vault формате
//     "vault:v<N>:<base64>". Vault Transit поддерживает rotation —
//     ciphertext'ы старых версий остаются readable после rotate, новые
//     шифруются текущей версией. Никакой кастомной "версии" поверх не
//     добавляем — Vault уже всё учитывает.
//   - Searchable hash (HMAC-SHA256): для equals-lookup'ов (например, "найти
//     applicant'а по СНИЛС") детерминированный hash считается отдельно
//     через EncryptHash. Hash считается локально с derived key — это НЕ
//     deterministic encryption (decryption невозможен), но достаточен для
//     индексирования по hash-колонке.
//
// Контракт безопасности:
//
//   - Plaintext НИКОГДА не логируется и не возвращается в ошибках.
//   - Empty plaintext → empty ciphertext (не лезем в Vault, не считаем hash).
//   - Юрлицо ИНН (10 цифр) НЕ шифруется — это публичный идентификатор по
//     ЕГРЮЛ. Шифруется только personal-inn (12 цифр, физлицо).
package piiencryption

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// FieldType — типизированный enum поддерживаемых PII полей. Каждое значение
// мапится на Vault Transit-ключ через KeyName(). Расширяется добавлением
// нового const + записи в supportedFields.
type FieldType string

const (
	// FieldPassportNumber — серия+номер паспорта РФ (формат "1234 567890").
	FieldPassportNumber FieldType = "passport-number"
	// FieldPassportIssuer — кем выдан паспорт (свободный текст).
	FieldPassportIssuer FieldType = "passport-issuer"
	// FieldSNILS — 11 цифр СНИЛС (формат "123-456-789 01").
	FieldSNILS FieldType = "snils"
	// FieldPersonalINN — ИНН физлица (12 цифр). Юрлицо (10 цифр) НЕ
	// шифруется — это публичный идентификатор.
	FieldPersonalINN FieldType = "personal-inn"
	// FieldBankAccount — расчётный счёт клиента (опционально, по 152-ФЗ).
	FieldBankAccount FieldType = "bank-account"
	// FieldPhone — номер телефона. Шифруется в случаях с регуляторными
	// требованиями (контрагент, бенефициар).
	FieldPhone FieldType = "phone"
)

// supportedFields — whitelist допустимых FieldType. Любой fieldType вне
// этого набора → ErrUnknownField. Добавление нового поля требует:
//  1. const FieldXxx FieldType = "xxx";
//  2. запись здесь;
//  3. Vault Transit-ключ "pii-xxx" должен быть создан/EnsureKey'нут на
//     старте сервиса.
var supportedFields = map[FieldType]struct{}{
	FieldPassportNumber: {},
	FieldPassportIssuer: {},
	FieldSNILS:          {},
	FieldPersonalINN:    {},
	FieldBankAccount:    {},
	FieldPhone:          {},
}

// Vault Transit key prefix для PII-полей.
const piiKeyPrefix = "pii-"

// KeyName возвращает имя Vault Transit-ключа для FieldType. Соглашение:
// "pii-<field>". Ключи изолированы per-field (разный blast radius).
func KeyName(f FieldType) string {
	return piiKeyPrefix + string(f)
}

// SupportedFields возвращает копию набора поддерживаемых FieldType. Удобно
// для bootstrap'а сервиса (EnsureKey по каждому полю).
func SupportedFields() []FieldType {
	out := make([]FieldType, 0, len(supportedFields))
	for f := range supportedFields {
		out = append(out, f)
	}
	return out
}

// IsSupported проверяет, что fieldType находится в whitelist'е.
func IsSupported(f FieldType) bool {
	_, ok := supportedFields[f]
	return ok
}

// Sentinel errors. Возвращаются обёрнутыми через fmt.Errorf("...: %w", ...).
var (
	// ErrUnknownField — fieldType не в supportedFields.
	ErrUnknownField = errors.New("piiencryption: unknown field type")
	// ErrEmptyPlaintext — внутренний sentinel; пустой plaintext не считается
	// ошибкой на API-уровне (Encrypt возвращает "", nil).
	ErrEmptyPlaintext = errors.New("piiencryption: empty plaintext")
)

// PIIEncryptor — высокоуровневый интерфейс шифрования PII полей. Прячет
// детали Vault Transit (key naming, base64-форматы) за типизированным
// FieldType. Repository-слой / domain-сервисы зависят только от него.
type PIIEncryptor interface {
	// Encrypt шифрует plaintext под ключом FieldType. Пустой plaintext
	// возвращает "", nil (не лезем в Vault). Ciphertext в формате
	// "vault:v<N>:<base64>" — версионирован Vault'ом, можно класть в БД.
	Encrypt(ctx context.Context, fieldType FieldType, plaintext string) (string, error)
	// Decrypt расшифровывает ciphertext (любой версии — Vault Transit
	// поддерживает старые версии после rotation). Пустой ciphertext → "", nil.
	Decrypt(ctx context.Context, fieldType FieldType, ciphertext string) (string, error)
	// EncryptHash возвращает HMAC-SHA256 от plaintext под derived-ключом
	// fieldType. Детерминирован (одинаковый plaintext → одинаковый hash) —
	// можно класть в индексированную колонку и искать equals'ом. НЕ
	// обратимо: не путать с deterministic encryption.
	EncryptHash(ctx context.Context, fieldType FieldType, plaintext string) (string, error)
}

// validateField — узкое место для проверки whitelist'а.
func validateField(f FieldType) error {
	if !IsSupported(f) {
		return fmt.Errorf("%w: %q", ErrUnknownField, f)
	}
	return nil
}

// computeHMAC — общая утилита для EncryptHash. derivationSecret служит
// "корнем" для derived-ключей: hashKey = HMAC(derivationSecret, fieldType).
// Это даёт per-field hash изоляцию: компрометация СНИЛС-словаря не
// раскрывает паспортный.
//
// Возвращает hex-encoded HMAC (64 символа). Префикс "h1:" зарезервирован
// для будущей миграции (h2: → другой алгоритм).
func computeHMAC(derivationSecret []byte, fieldType FieldType, plaintext string) string {
	// Derive per-field hash key.
	keyMac := hmac.New(sha256.New, derivationSecret)
	keyMac.Write([]byte(KeyName(fieldType)))
	derivedKey := keyMac.Sum(nil)

	// HMAC over plaintext with derived key.
	mac := hmac.New(sha256.New, derivedKey)
	mac.Write([]byte(plaintext))
	return "h1:" + hex.EncodeToString(mac.Sum(nil))
}

// Package signature определяет абстракцию УКЭП (Усиленной Квалифицированной
// Электронной Подписи) по ГОСТ Р 34.10-2012 / Р 34.11-2012 (Streebog).
//
// Покрытие:
//   - SignatureProvider — интерфейс СКЗИ-провайдера (КриптоПро / VipNet / Mock).
//   - SignedPayload — результат подписания (immutable, audit-friendly).
//   - Certificate — описание сертификата (subject, validity, thumbprint).
//   - VerificationResult — результат проверки SignedPayload.
//
// **Не привязан к конкретному СКЗИ**. Real КриптоПро/VipNet провайдеры —
// отдельный пакет (signature/cryptopro, signature/vipnet), которые подключаются
// через ENV/Vault после получения СКЗИ-лицензий (см. docs/pilot-readiness.md § 1.4).
//
// Сейчас в репо только Mock-имплементация — для unit/integration-тестов
// document-service / identity-service / audit-service.
//
// ADR-ы:
//   - docs/adr/0010-billing-and-audit-log.md (audit signature concept)
//   - docs/security-architecture.md § 6.3 (УКЭП в архитектуре безопасности)
package signature

import (
	"context"
	"errors"
	"time"
)

// ErrCertificateExpired — сертификат истёк, подписание невозможно.
var ErrCertificateExpired = errors.New("signature: certificate expired")

// ErrCertificateNotFound — указанный certID не найден в СКЗИ.
var ErrCertificateNotFound = errors.New("signature: certificate not found")

// ErrInvalidSignature — verification failed (signature mismatch / hash mismatch / etc.).
var ErrInvalidSignature = errors.New("signature: invalid")

// ErrProviderNotConfigured — провайдер не сконфигурирован (например, mock без stub-сертификатов).
var ErrProviderNotConfigured = errors.New("signature: provider not configured")

// SignatureProvider — абстракция СКЗИ-провайдера для подписания/верификации.
//
// Контракт:
//   - Sign(payload, certID) → SignedPayload или error. Provider сам выбирает
//     алгоритм хеширования (по умолчанию Streebog-256 для ГОСТ-2012).
//   - Verify(signed) → результат с признаком валидности и metadata подписавшего.
//   - ListCertificates / GetCertificate — для UI выбора сертификата клиентом.
//
// Конкурентная безопасность: реализации должны быть thread-safe (provider
// обычно используется shared между HTTP handler'ами).
type SignatureProvider interface {
	// Sign подписывает payload, используя сертификат с указанным certID.
	// Возвращает SignedPayload, готовый к сериализации в JSON и хранению в БД.
	Sign(ctx context.Context, payload []byte, certID string) (*SignedPayload, error)

	// Verify проверяет SignedPayload: валидность подписи против payload-hash,
	// валидность сертификата (не истёк), опционально chain до корневого УЦ.
	Verify(ctx context.Context, signed *SignedPayload) (*VerificationResult, error)

	// ListCertificates возвращает все доступные сертификаты в СКЗИ.
	// Используется UI для отображения списка выбора.
	ListCertificates(ctx context.Context) ([]*Certificate, error)

	// GetCertificate возвращает полный сертификат по ID (для отправки клиенту
	// или для verification chain).
	GetCertificate(ctx context.Context, certID string) (*Certificate, error)

	// Name возвращает имя реализации ("cryptopro", "vipnet", "mock").
	// Используется для logging и audit-emit.
	Name() string
}

// SignedPayload — результат подписания. Хранится в БД (как JSONB) или передаётся
// между сервисами по сети. Поля выбраны так, чтобы Verify был возможен без
// доступа к исходному payload (только хеш + signature + cert).
type SignedPayload struct {
	// Signature — DER-encoded ГОСТ-подпись (Р 34.10-2012). Для Mock — детерминированный hash.
	Signature []byte `json:"signature"`

	// PayloadHash — hex-encoded хеш исходного payload (Streebog-256/512 или SHA-256
	// в Mock-режиме). Используется для re-verification без хранения raw payload.
	PayloadHash string `json:"payload_hash"`

	// HashAlgorithm — "streebog256", "streebog512", "sha256" (для Mock).
	HashAlgorithm string `json:"hash_algorithm"`

	// SignerCertificate — PEM/DER-encoded X.509 сертификат подписавшего (hex-encoded в JSON).
	SignerCertificate string `json:"signer_certificate"`

	// CertificateChain — опциональная цепочка до корня УЦ (для chain validation).
	CertificateChain []string `json:"certificate_chain,omitempty"`

	// SignedAt — UTC timestamp подписания (ISO 8601 при сериализации).
	SignedAt time.Time `json:"signed_at"`

	// Provider — имя СКЗИ ("cryptopro", "vipnet", "mock").
	Provider string `json:"provider"`

	// ProviderVersion — версия СКЗИ (для audit trail и forensics).
	ProviderVersion string `json:"provider_version,omitempty"`

	// Container — опциональный CMS/CAdES container (если СКЗИ поддерживает).
	Container []byte `json:"container,omitempty"`
}

// VerificationResult — итог Verify. Поле Valid — основной критерий, остальные
// поля — для audit log и UI ("кто подписал?").
type VerificationResult struct {
	// Valid — основной флаг: подпись математически валидна и сертификат не истёк.
	Valid bool `json:"valid"`

	// SignerName — Common Name (CN) подписавшего из сертификата.
	SignerName string `json:"signer_name,omitempty"`

	// SignerINN — ИНН из сертификата (extension OID 1.2.643.3.131.1.1).
	SignerINN string `json:"signer_inn,omitempty"`

	// SignerSNILS — СНИЛС из сертификата (extension OID 1.2.643.100.3).
	SignerSNILS string `json:"signer_snils,omitempty"`

	// CertificateExpiry — когда истекает сертификат (для UI warning).
	CertificateExpiry time.Time `json:"certificate_expiry,omitempty"`

	// IsExpired — истёк ли сертификат на момент verification.
	IsExpired bool `json:"is_expired"`

	// ChainValid — валидна ли цепочка сертификатов (если предоставлена).
	ChainValid bool `json:"chain_valid,omitempty"`

	// TimestampVerified — проверен ли TSA-таймстамп (если применимо).
	TimestampVerified bool `json:"timestamp_verified,omitempty"`

	// ErrorMessage — человеко-читаемая причина невалидности.
	ErrorMessage string `json:"error,omitempty"`
}

// Certificate — описание сертификата (для UI choice + verification chain).
//
// PrivateKey никогда не возвращается из СКЗИ — он остаётся внутри криптомодуля.
// Это field-by-design нет в этой структуре.
type Certificate struct {
	// ID — идентификатор для Sign() (serial number, thumbprint или UUID — зависит от провайдера).
	ID string `json:"id"`

	// Subject — DN из X.509: "CN=Иванов И.И., O=Demo Bank, OU=Compliance, C=RU".
	Subject string `json:"subject"`

	// SerialNumber — hex-encoded.
	SerialNumber string `json:"serial_number"`

	// Issuer — DN издавшего УЦ.
	Issuer string `json:"issuer"`

	// ValidFrom / ValidTo — период действия (UTC).
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`

	// Thumbprint — SHA-256 fingerprint, hex-encoded.
	Thumbprint string `json:"thumbprint"`

	// AlgorithmOID — OID алгоритма ключа (1.2.643.2.2.3 для ГОСТ-2001;
	// 1.2.643.7.1.1.1 для ГОСТ-2012/256; 1.2.643.7.1.1.2 для ГОСТ-2012/512).
	AlgorithmOID string `json:"algorithm_oid"`

	// KeySize — длина ключа в bit (256 / 512 для ГОСТ).
	KeySize int `json:"key_size"`

	// IsExpired — истёк ли на момент запроса.
	IsExpired bool `json:"is_expired"`

	// PublicKey — DER-encoded публичный ключ (hex-encoded в JSON).
	PublicKey string `json:"public_key,omitempty"`
}

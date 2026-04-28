// Package gost2012 содержит STUB-имплементацию подписанта,
// маркированного алгоритмом "gost-2012-256-stub".
//
// !!! ВНИМАНИЕ !!!
//
// Это НЕ реальный ГОСТ Р 34.10-2012 / Streebog (Р 34.11-2012).
// Внутри используется SHA-256 как hash-функция и keyed-SHA-256 как
// "подпись". Длина выходной "подписи" совпадает с ГОСТ-2012-256
// (32 байта), но криптографические свойства принципиально иные —
// это **не имитация криптостойкости**, а инфраструктурный placeholder.
//
// Цели stub-реализации:
//
//  1. Дать audit-service, document-service и другим консьюмерам
//     возможность писать события с algorithm-маркером
//     "gost-2012-256-stub" в on-prem dev/staging-инсталляциях
//     российских банков, у которых ed25519 не принимается комплаенсом
//     "по визуальной идентичности с ГОСТ".
//  2. Позволить QA отрепетировать flow swap'а на real КриптоПро
//     (см. roadmap: packages/signature/cryptopro после получения
//     лицензий, lead-time 2-4 недели).
//  3. Сделать любое случайное использование stub-данных в production
//     **громким и обнаруживаемым** — algorithm-поле "gost-2012-256-stub"
//     несовместимо с целевым "gost-2012-256", поэтому verifier при
//     production-rollout сразу отклонит stub-payload'ы.
//
// !!! НЕ ИСПОЛЬЗОВАТЬ В PRODUCTION !!!
//
// Свойства stub'а:
//   - Sign(digest) → SHA-256(privateKey || digest), 32 байта.
//   - Verify(digest, sig) → recompute и compare (constant-time не гарантируется).
//   - KeyID() → "STUB-" + hex(SHA-256(publicKey))[:32], явно префиксован.
//   - Provider/Algorithm маркер: "gost-2012-256-stub" (ОТЛИЧАЕТСЯ от целевого
//     "gost-2012-256" — это намеренно, см. п.3 выше).
//
// Путь миграции:
//   - Заменить импорт consumer-сервисов с
//     "packages/signature/gost2012" на "packages/signature/cryptopro"
//     после получения лицензий.
//   - Удалить эту директорию (или оставить только в dev-builds под build-tag).
//
// Связанные документы:
//   - docs/security-architecture.md § 6.3 — требования УКЭП по ГОСТ-2012.
//   - docs/compliance-map.md — соответствие ФСБ-сертификации.
//   - packages/signature/ed25519 — реально работающий compact signer для audit.
package gost2012

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// Algorithm — маркер, записываемый consumer'ами в audit.signature_algorithm
// или в SignedPayload.HashAlgorithm. Намеренно отличается от целевого
// "gost-2012-256", чтобы stub-данные не путались с реальной ГОСТ-подписью
// при production rollout (verifier отклонит mismatch на etапе разбора).
const Algorithm = "gost-2012-256-stub"

// KeySize — размер "приватного ключа" stub'а в байтах. Совпадает с длиной
// closed key ГОСТ-2012-256 для visual identification и для удобной замены
// в Vault при swap'е на реальный СКЗИ.
const KeySize = 32

// SignatureSize — длина выходной "подписи" (соответствует ГОСТ-2012-256
// по формату, но НЕ по криптостойкости — внутри это SHA-256 digest).
const SignatureSize = 32

// keyIDPrefix — обязательный префикс для KeyID(), позволяет аудит-сервисам
// и SOC-аналитикам отфильтровать stub-события grep'ом по "STUB-".
const keyIDPrefix = "STUB-"

// StubSigner — placeholder-подписант с алгоритмом "gost-2012-256-stub".
// Безопасен для concurrent использования (sha256.Sum — pure-функция).
//
// !!! Не криптографически стойкий. Только для dev/staging. !!!
type StubSigner struct {
	priv      []byte // 32 байта random — visual-identification stub-key
	pub       []byte // 32 байта = SHA-256(priv) — derived "public", чтобы
	                 // round-trip от приватного ключа был детерминирован
	keyID     string // "STUB-" + hex(SHA-256(pub))[:32]
	pubBase64 string
}

// GenerateStub создаёт новую stub-keypair через crypto/rand. Используется
// для dev-окружения при старте сервиса (НЕ для production — там вообще
// не должно быть gost2012, а должен быть cryptopro).
func GenerateStub() (*StubSigner, error) {
	priv := make([]byte, KeySize)
	if _, err := rand.Read(priv); err != nil {
		return nil, fmt.Errorf("gost2012-stub: generate random key: %w", err)
	}
	return newFromPriv(priv), nil
}

// FromStubKey создаёт StubSigner из 32-байтного raw private key.
//
// Detereminstic: одинаковый key → одинаковый KeyID → одинаковая подпись.
// Это позволяет хранить stub-key в Vault как короткую строку и
// reconstruct'ить полный signer при старте сервиса.
func FromStubKey(priv []byte) (*StubSigner, error) {
	if len(priv) != KeySize {
		return nil, fmt.Errorf("gost2012-stub: invalid private key size %d (want %d)",
			len(priv), KeySize)
	}
	cp := make([]byte, KeySize)
	copy(cp, priv)
	return newFromPriv(cp), nil
}

// FromBase64Stub — convenience-обёртка для ENV-переменных и Vault-secret.
// Принимает base64(32 bytes).
func FromBase64Stub(b64 string) (*StubSigner, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("gost2012-stub: decode base64 key: %w", err)
	}
	return FromStubKey(raw)
}

// FromEnv читает env-переменную (base64 32-байтного stub-key). Если пусто
// или переменная не задана — возвращает (nil, nil) для backwards-compat
// поведения "no signer configured → audit пишет события без подписи".
func FromEnv(envName string) (*StubSigner, error) {
	v := os.Getenv(envName)
	if v == "" {
		return nil, nil
	}
	return FromBase64Stub(v)
}

// Sign возвращает 32-байтную stub-"подпись" вида SHA-256(priv || digest).
//
// !!! Это НЕ ГОСТ Р 34.10-2012. Это keyed-SHA-256 placeholder. !!!
//
// Длина (32 байта) совпадает с ГОСТ-2012-256 для visual-identification
// в audit-логе и для упрощения миграции на реальный СКЗИ.
func (s *StubSigner) Sign(digest []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("gost2012-stub: nil signer")
	}
	sum := computeStubSignature(s.priv, digest)
	out := make([]byte, SignatureSize)
	copy(out, sum[:])
	return out, nil
}

// Verify проверяет stub-подпись через recompute + compare. Не constant-time
// (это не security-сенситивная операция — stub есть stub).
func (s *StubSigner) Verify(digest, signature []byte) error {
	if s == nil {
		return errors.New("gost2012-stub: nil signer")
	}
	if len(signature) != SignatureSize {
		return fmt.Errorf("gost2012-stub: invalid signature size %d (want %d)",
			len(signature), SignatureSize)
	}
	expected := computeStubSignature(s.priv, digest)
	for i := 0; i < SignatureSize; i++ {
		if expected[i] != signature[i] {
			return errors.New("gost2012-stub: signature verification failed")
		}
	}
	return nil
}

// KeyID — стабильный идентификатор подписанта.
//
// Формат: "STUB-" + hex(SHA-256(pub))[:32].
//
// Префикс "STUB-" обязателен — позволяет SOC и audit-аналитикам
// отфильтровать stub-события одним grep'ом и не смешать их с реальной
// ГОСТ-подписью.
func (s *StubSigner) KeyID() string { return s.keyID }

// PublicKey возвращает 32-байтный "публичный ключ" stub'а (SHA-256(priv)).
// Это derived-значение, не настоящий public key (у симметричного stub'а
// настоящего public key нет).
func (s *StubSigner) PublicKey() []byte {
	cp := make([]byte, len(s.pub))
	copy(cp, s.pub)
	return cp
}

// PublicKeyBase64 — base64-encoded "public key" (32 байта). Используется
// verifier'ом, который не имеет доступа к приватной части.
//
// !!! Внимание: для stub'а Verify требует тот же priv, что и Sign — у
// симметричного stub'а нет асимметричной verify-роли. PublicKeyBase64
// существует только для совместимости API с ed25519-пакетом и для
// будущей замены на реальный КриптоПро. !!!
func (s *StubSigner) PublicKeyBase64() string { return s.pubBase64 }

// KeyIDFromPublicKey — same algorithm как StubSigner.KeyID(),
// но для verifier-side вычисления (когда есть только pub).
func KeyIDFromPublicKey(pub []byte) string {
	sum := sha256.Sum256(pub)
	return keyIDPrefix + hex.EncodeToString(sum[:])[:32]
}

// PublicKeyFromBase64 распаковывает base64-encoded "public key".
func PublicKeyFromBase64(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("gost2012-stub: decode base64 pubkey: %w", err)
	}
	if len(raw) != KeySize {
		return nil, fmt.Errorf("gost2012-stub: invalid pubkey size %d (want %d)",
			len(raw), KeySize)
	}
	return raw, nil
}

func newFromPriv(priv []byte) *StubSigner {
	pubSum := sha256.Sum256(priv)
	pub := pubSum[:]
	return &StubSigner{
		priv:      priv,
		pub:       pub,
		keyID:     KeyIDFromPublicKey(pub),
		pubBase64: base64.StdEncoding.EncodeToString(pub),
	}
}

// computeStubSignature — внутренняя "псевдо-подпись" SHA-256(priv || digest).
// Намеренно простая функция, читается с первого взгляда — чтобы никто
// не подумал, что это настоящий ГОСТ.
func computeStubSignature(priv, digest []byte) [32]byte {
	buf := make([]byte, 0, len(priv)+len(digest))
	buf = append(buf, priv...)
	buf = append(buf, digest...)
	return sha256.Sum256(buf)
}

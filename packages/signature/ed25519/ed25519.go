// Package ed25519 предоставляет компактную обёртку над crypto/ed25519
// для подписания audit-event'ов и других compact-данных.
//
// Используется в audit-service до получения СКЗИ-лицензий
// (КриптоПро/VipNet с ГОСТ-2012). После подключения СКЗИ — алгоритм
// в БД меняется на "gost-2012-256", старые ed25519-события остаются
// валидными за счёт SignatureAlgorithm-поля.
//
// Контракт:
//   - Signer.Sign(digest) → 64-byte ed25519 signature
//   - Signer.Verify(digest, signature) → error
//   - Signer.KeyID() → детерминированный fingerprint (hex SHA-256 от
//     PublicKey), используется в audit.signer_key_id колонке.
//
// Безопасность:
//   - Приватный ключ хранится в памяти процесса. В production должен
//     приходить из Vault через packages/secrets, не из ENV (для prod).
//     В dev — env-переменная AUDIT_SIGNING_KEY (base64 32 bytes seed)
//     или генерация при старте.
//   - Verifier'у нужен только PublicKey — не приватный.
package ed25519

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
)

// Algorithm — значение, записываемое в audit.signature_algorithm колонку
// и проверяемое verifier'ом для выбора правильного verify-алгоритма.
const Algorithm = "ed25519"

// SeedSize — размер seed для GenerateKey (NewKeyFromSeed).
const SeedSize = ed25519.SeedSize

// Signer оборачивает private key с pre-computed key-id и public-key.
// Безопасен для concurrent использования (ed25519.Sign — pure-функция).
type Signer struct {
	priv      ed25519.PrivateKey
	pub       ed25519.PublicKey
	keyID     string
	pubBase64 string
}

// New создаёт Signer из ed25519 private key (32 byte seed → 64 byte priv).
func New(priv ed25519.PrivateKey) (*Signer, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("ed25519: invalid private key size %d (want %d)",
			len(priv), ed25519.PrivateKeySize)
	}
	pub := priv.Public().(ed25519.PublicKey)
	return &Signer{
		priv:      priv,
		pub:       pub,
		keyID:     computeKeyID(pub),
		pubBase64: base64.StdEncoding.EncodeToString(pub),
	}, nil
}

// FromSeed создаёт Signer из 32-байтного seed.
//
// Deterministic: одинаковый seed → одинаковый ключ → одинаковый KeyID.
// Это позволяет хранить seed в Vault как короткую строку (32 bytes) и
// reconstruct полный ключ при старте сервиса.
func FromSeed(seed []byte) (*Signer, error) {
	if len(seed) != SeedSize {
		return nil, fmt.Errorf("ed25519: invalid seed size %d (want %d)",
			len(seed), SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return New(priv)
}

// FromBase64Seed — convenience-обёртка для ENV-переменных и Vault secrets.
func FromBase64Seed(b64 string) (*Signer, error) {
	seed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("ed25519: decode base64 seed: %w", err)
	}
	return FromSeed(seed)
}

// FromEnv читает AUDIT_SIGNING_KEY (base64 32-byte seed). Если пусто или
// переменная не задана — возвращает (nil, nil). Это позволяет audit-service
// работать без подписи (backwards compat) при пустом env.
func FromEnv(envName string) (*Signer, error) {
	v := os.Getenv(envName)
	if v == "" {
		return nil, nil
	}
	return FromBase64Seed(v)
}

// Generate создаёт новую keypair через crypto/rand. Используется для
// dev-окружения при старте сервиса (не для production — там seed из Vault).
func Generate() (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ed25519: generate: %w", err)
	}
	return &Signer{
		priv:      priv,
		pub:       pub,
		keyID:     computeKeyID(pub),
		pubBase64: base64.StdEncoding.EncodeToString(pub),
	}, nil
}

// Sign подписывает digest и возвращает 64-байтную подпись.
//
// Для audit-event digest — это hex-decoded SHA-256 hash события
// (см. domain.AuditEvent.SignedDigest()).
func (s *Signer) Sign(digest []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("ed25519: nil signer")
	}
	return ed25519.Sign(s.priv, digest), nil
}

// Verify проверяет подпись против digest и встроенного PublicKey.
func (s *Signer) Verify(digest, signature []byte) error {
	if s == nil {
		return errors.New("ed25519: nil signer")
	}
	return VerifyWithPublicKey(s.pub, digest, signature)
}

// KeyID — стабильный hex-encoded SHA-256 fingerprint от PublicKey.
// Используется в audit.signer_key_id для соотнесения подписи с ключом
// при verification.
func (s *Signer) KeyID() string { return s.keyID }

// PublicKey возвращает встроенный public key (для экспорта верификатору).
func (s *Signer) PublicKey() ed25519.PublicKey { return s.pub }

// PublicKeyBase64 — base64-encoded public key (32 bytes), удобно для
// передачи verifier'у через CLI flag или config-файл.
func (s *Signer) PublicKeyBase64() string { return s.pubBase64 }

// VerifyWithPublicKey — stand-alone verification без Signer (для verifier-CLI).
func VerifyWithPublicKey(pub ed25519.PublicKey, digest, signature []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("ed25519: invalid public key size %d", len(pub))
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("ed25519: invalid signature size %d (want %d)",
			len(signature), ed25519.SignatureSize)
	}
	if !ed25519.Verify(pub, digest, signature) {
		return errors.New("ed25519: signature verification failed")
	}
	return nil
}

// PublicKeyFromBase64 распаковывает base64-encoded public key.
func PublicKeyFromBase64(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("ed25519: decode base64 pubkey: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ed25519: invalid pubkey size %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// KeyIDFromPublicKey — same algorithm как Signer.KeyID(), но для verifier-side.
func KeyIDFromPublicKey(pub ed25519.PublicKey) string {
	return computeKeyID(pub)
}

func computeKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

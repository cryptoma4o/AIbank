package piiencryption

import (
	"context"
	"errors"
	"fmt"

	"github.com/aibank/platform/packages/secrets"
)

// VaultPIIEncryptor — реализация PIIEncryptor через secrets.Transit
// (HashiCorp Vault Transit Engine). Тонкая обёртка: маппинг FieldType →
// Vault key + проверка whitelist'а.
//
// Ciphertext формат — нативный Vault: "vault:v<N>:<base64>". Versioned
// автоматически: rotate ключа в Vault → новые шифрования идут под v<N+1>,
// старые ciphertext'ы продолжают читаться.
//
// HashSecret — derivation root для EncryptHash. Должен быть стабилен между
// рестартами сервиса (иначе hash'и в БД станут невалидными). Берётся из
// secrets.Provider (Vault KV) на старте сервиса. Минимум 32 байта.
type VaultPIIEncryptor struct {
	transit    secrets.Transit
	hashSecret []byte
}

// VaultPIIConfig — конфигурация конструктора.
type VaultPIIConfig struct {
	// Transit — клиент Vault Transit. Обязательный.
	Transit secrets.Transit
	// HashSecret — derivation root для HMAC-SHA256 (>= 32 байта).
	// Получается из Vault KV на старте сервиса (см. secrets.Provider).
	// НЕ хранить в коде / env vars в проде.
	HashSecret []byte
}

const minHashSecretLen = 32

// NewVaultPIIEncryptor валидирует cfg и собирает encryptor. Ленивый — в
// Vault на этапе конструктора не ходит. EnsureKeys вызывать отдельно на
// bootstrap'е.
func NewVaultPIIEncryptor(cfg VaultPIIConfig) (*VaultPIIEncryptor, error) {
	if cfg.Transit == nil {
		return nil, errors.New("piiencryption: Transit client is required")
	}
	if len(cfg.HashSecret) < minHashSecretLen {
		return nil, fmt.Errorf("piiencryption: HashSecret must be >= %d bytes", minHashSecretLen)
	}
	// Defensive copy — caller может зануллить slice.
	secretCopy := make([]byte, len(cfg.HashSecret))
	copy(secretCopy, cfg.HashSecret)
	return &VaultPIIEncryptor{
		transit:    cfg.Transit,
		hashSecret: secretCopy,
	}, nil
}

// EnsureKeys идемпотентно создаёт все Transit-ключи для поддерживаемых
// FieldType'ов. Вызывать на старте сервиса (один раз). Если ключ уже
// существует в Vault — no-op (поведение Transit engine).
func (v *VaultPIIEncryptor) EnsureKeys(ctx context.Context) error {
	for _, f := range SupportedFields() {
		if err := v.transit.EnsureKey(ctx, KeyName(f)); err != nil {
			return fmt.Errorf("piiencryption: ensure key for %s: %w", f, err)
		}
	}
	return nil
}

// Encrypt реализует PIIEncryptor.
func (v *VaultPIIEncryptor) Encrypt(ctx context.Context, fieldType FieldType, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	ct, err := v.transit.Encrypt(ctx, KeyName(fieldType), plaintext)
	if err != nil {
		return "", fmt.Errorf("piiencryption: encrypt %s: %w", fieldType, err)
	}
	return ct, nil
}

// Decrypt реализует PIIEncryptor.
func (v *VaultPIIEncryptor) Decrypt(ctx context.Context, fieldType FieldType, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	pt, err := v.transit.Decrypt(ctx, KeyName(fieldType), ciphertext)
	if err != nil {
		return "", fmt.Errorf("piiencryption: decrypt %s: %w", fieldType, err)
	}
	return pt, nil
}

// EncryptHash реализует PIIEncryptor. Локальный HMAC-SHA256 — в Vault не ходит.
func (v *VaultPIIEncryptor) EncryptHash(_ context.Context, fieldType FieldType, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	return computeHMAC(v.hashSecret, fieldType, plaintext), nil
}

// Compile-time assertion.
var _ PIIEncryptor = (*VaultPIIEncryptor)(nil)

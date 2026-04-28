package piiencryption

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

// MockPIIEncryptor — детерминистический "шифратор" для тестов.
// Реализует PIIEncryptor поверх XOR-маски, без обращения к сети.
//
// Свойства:
//   - Encrypt/Decrypt round-trip (Decrypt(Encrypt(x)) == x).
//   - Encrypt детерминирован — одинаковый plaintext+fieldType дают
//     одинаковый ciphertext (это важно для assert'ов в тестах, в проде
//     Vault Transit рандомизирует nonce).
//   - Per-field изоляция: ciphertext'ы для одного и того же plaintext под
//     разными FieldType отличаются (разная XOR-маска).
//   - EncryptHash использует фиксированный mock-secret и тот же computeHMAC,
//     что и реальный VaultPIIEncryptor — h1: префикс совпадает.
//
// ВНИМАНИЕ: НЕ криптография. ТОЛЬКО для unit-тестов и dev-сценариев.
type MockPIIEncryptor struct {
	mu        sync.Mutex
	callCount int
}

const mockPrefix = "vault:v1:mock-"

// mockHashSecret — стабильный 32-байтный seed для EncryptHash в тестах.
// Не используется в проде (VaultPIIEncryptor требует HashSecret из Vault).
var mockHashSecret = []byte("mock-pii-hash-secret-32-bytes!!!")

// NewMockPIIEncryptor — конструктор.
func NewMockPIIEncryptor() *MockPIIEncryptor {
	return &MockPIIEncryptor{}
}

// CallCount — диагностический счётчик для тестов (сколько раз вызвали
// Encrypt/Decrypt/EncryptHash суммарно).
func (m *MockPIIEncryptor) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *MockPIIEncryptor) bump() {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
}

// Encrypt применяет XOR-маску из KeyName(fieldType) и base64-кодирует
// результат с префиксом "vault:v1:mock-" (имитирует Vault-формат для
// совместимости тестов с production-парсерами).
func (m *MockPIIEncryptor) Encrypt(_ context.Context, fieldType FieldType, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	m.bump()
	masked := xorMask([]byte(plaintext), []byte(KeyName(fieldType)))
	return mockPrefix + base64.StdEncoding.EncodeToString(masked), nil
}

// Decrypt — обратная XOR-операция.
func (m *MockPIIEncryptor) Decrypt(_ context.Context, fieldType FieldType, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	if !strings.HasPrefix(ciphertext, mockPrefix) {
		return "", fmt.Errorf("piiencryption/mock: not a mock ciphertext for %s", fieldType)
	}
	m.bump()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, mockPrefix))
	if err != nil {
		return "", fmt.Errorf("piiencryption/mock: bad base64: %w", err)
	}
	return string(xorMask(raw, []byte(KeyName(fieldType)))), nil
}

// EncryptHash — тот же HMAC, что и в проде, но с фиксированным mockHashSecret.
// Детерминирован между тестами (стабильные assert'ы).
func (m *MockPIIEncryptor) EncryptHash(_ context.Context, fieldType FieldType, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateField(fieldType); err != nil {
		return "", err
	}
	m.bump()
	return computeHMAC(mockHashSecret, fieldType, plaintext), nil
}

func xorMask(data, key []byte) []byte {
	if len(key) == 0 {
		return data
	}
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ key[i%len(key)]
	}
	return out
}

// Compile-time assertion.
var _ PIIEncryptor = (*MockPIIEncryptor)(nil)

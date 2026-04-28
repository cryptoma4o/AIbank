package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

// Field-level encryption через Vault Transit (см. docs/security-architecture.md
// § 5.2). Шифруется поверх TDE: серия/номер паспорта, СНИЛС, полный ИНН и
// прочие поля, чувствительные к компрометации БД.
//
// Архитектура ключей (см. § 6.2): master-key (HSM / sealed Vault) → tenant-key
// → data-key. На уровне приложения мы оперируем tenant-keys по имени
// "aibank-tenant-<tenant_id>" — каждый тенант изолирован своим ключом, что
// упрощает выгрузку (re-wrap) и удаление при offboarding'е.

// TenantKeyPrefix — префикс имени Transit-ключа для пер-тенантной изоляции.
// Полное имя формирует TenantKeyName().
const TenantKeyPrefix = "aibank-tenant-"

// TenantKeyName собирает имя Transit-ключа для тенанта по соглашению
// "aibank-tenant-<tenant_id>". tenantID должен быть уже валидирован вызывающей
// стороной (lower-case, ascii, см. валидаторы repository-слоя).
func TenantKeyName(tenantID string) string {
	return TenantKeyPrefix + tenantID
}

// TransitClient — клиент Vault Transit Engine. Минимальный поверх vault-api:
// Encrypt / Decrypt / EnsureKey. Ciphertext возвращается в нативном формате
// Vault — строка вида "vault:v1:<base64>" — её нужно класть в BYTEA поле БД.
type TransitClient struct {
	cfg    TransitConfig
	client *vaultapi.Client

	mu       sync.Mutex
	knownKey map[string]struct{} // кэш "ключ уже создан" для EnsureKey
}

// TransitConfig — конфигурация TransitClient.
//
// MountPath — путь Transit-engine'а в Vault (обычно "transit"). Полные API
// пути выглядят как "<mount>/encrypt/<key>", "<mount>/decrypt/<key>",
// "<mount>/keys/<key>".
type TransitConfig struct {
	// Address — VAULT_ADDR.
	Address string
	// Token — VAULT_TOKEN. В проде заменить на AppRole / Vault Agent (TODO).
	Token string
	// Namespace — Vault Enterprise namespace (опционально).
	Namespace string
	// MountPath — обычно "transit".
	MountPath string
	// RequestTimeout — per-call deadline (default 5 sec).
	RequestTimeout time.Duration
}

// NewTransitClient собирает клиента из cfg. Ленивый: ничего в Vault не
// дёргает; ошибки сети всплывут в Encrypt/Decrypt/EnsureKey.
func NewTransitClient(cfg TransitConfig) (*TransitClient, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("secrets/transit: VAULT_ADDR is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("secrets/transit: VAULT_TOKEN is required (AppRole TODO)")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "transit"
	}
	cfg.MountPath = strings.Trim(cfg.MountPath, "/")
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Second
	}

	apiCfg := vaultapi.DefaultConfig()
	apiCfg.Address = cfg.Address
	apiCfg.Timeout = cfg.RequestTimeout
	cli, err := vaultapi.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("secrets/transit: build client: %w", err)
	}
	cli.SetToken(cfg.Token)
	if cfg.Namespace != "" {
		cli.SetNamespace(cfg.Namespace)
	}
	return &TransitClient{
		cfg:      cfg,
		client:   cli,
		knownKey: make(map[string]struct{}),
	}, nil
}

// Transit — узкий интерфейс, под которым подписаны и реальный TransitClient,
// и MockTransit. Repository-слой зависит только от него — это позволяет
// прокидывать nil/mock в тестах без сети.
type Transit interface {
	Encrypt(ctx context.Context, keyName, plaintext string) (string, error)
	Decrypt(ctx context.Context, keyName, ciphertext string) (string, error)
	EnsureKey(ctx context.Context, keyName string) error
}

// Compile-time assertions.
var (
	_ Transit = (*TransitClient)(nil)
	_ Transit = (*MockTransit)(nil)
)

// Encrypt шифрует plaintext ключом keyName и возвращает Vault-формат
// ciphertext'а ("vault:v1:<base64>"). Пустой plaintext → возвращается
// пустая строка без обращения к Vault.
func (t *TransitClient) Encrypt(ctx context.Context, keyName, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateKeyName(keyName); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, t.cfg.RequestTimeout)
	defer cancel()

	path := t.cfg.MountPath + "/encrypt/" + keyName
	resp, err := t.client.Logical().WriteWithContext(cctx, path, map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString([]byte(plaintext)),
	})
	if err != nil {
		return "", fmt.Errorf("secrets/transit: encrypt %q: %w", keyName, err)
	}
	if resp == nil || resp.Data == nil {
		return "", fmt.Errorf("secrets/transit: encrypt %q: empty response", keyName)
	}
	ct, ok := resp.Data["ciphertext"].(string)
	if !ok || ct == "" {
		return "", fmt.Errorf("secrets/transit: encrypt %q: no ciphertext field", keyName)
	}
	return ct, nil
}

// Decrypt расшифровывает Vault-format ciphertext ключом keyName.
func (t *TransitClient) Decrypt(ctx context.Context, keyName, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := validateKeyName(keyName); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, t.cfg.RequestTimeout)
	defer cancel()

	path := t.cfg.MountPath + "/decrypt/" + keyName
	resp, err := t.client.Logical().WriteWithContext(cctx, path, map[string]interface{}{
		"ciphertext": ciphertext,
	})
	if err != nil {
		return "", fmt.Errorf("secrets/transit: decrypt %q: %w", keyName, err)
	}
	if resp == nil || resp.Data == nil {
		return "", fmt.Errorf("secrets/transit: decrypt %q: empty response", keyName)
	}
	b64, ok := resp.Data["plaintext"].(string)
	if !ok {
		return "", fmt.Errorf("secrets/transit: decrypt %q: no plaintext field", keyName)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("secrets/transit: decrypt %q: bad base64: %w", keyName, err)
	}
	return string(raw), nil
}

// EnsureKey идемпотентно создаёт Transit-ключ keyName с типом aes256-gcm96.
// Повторные вызовы для одного и того же ключа в рамках процесса не дёргают
// Vault — кэш в knownKey. Если ключ уже существует в Vault, второй вызов
// API вернёт 204/200 без ошибки (поведение Transit engine).
func (t *TransitClient) EnsureKey(ctx context.Context, keyName string) error {
	if err := validateKeyName(keyName); err != nil {
		return err
	}
	t.mu.Lock()
	if _, ok := t.knownKey[keyName]; ok {
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, t.cfg.RequestTimeout)
	defer cancel()
	path := t.cfg.MountPath + "/keys/" + keyName
	_, err := t.client.Logical().WriteWithContext(cctx, path, map[string]interface{}{
		"type":       "aes256-gcm96",
		"exportable": false,
	})
	if err != nil {
		return fmt.Errorf("secrets/transit: ensure key %q: %w", keyName, err)
	}

	t.mu.Lock()
	t.knownKey[keyName] = struct{}{}
	t.mu.Unlock()
	return nil
}

// EncryptOrEmpty — удобный wrapper: пустая строка проходит насквозь без
// ошибки. Используется в repository-слое чтобы не плодить if-ы для optional
// полей (СНИЛС, паспорт у нерезидентов и т.п.).
func EncryptOrEmpty(ctx context.Context, t Transit, keyName, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if t == nil {
		return value, nil
	}
	return t.Encrypt(ctx, keyName, value)
}

// DecryptOrEmpty — зеркальный helper для чтения. nil Transit означает, что
// dev-режим работает с TEXT-колонками без шифрования (см. README §
// "Field-level encryption").
func DecryptOrEmpty(ctx context.Context, t Transit, keyName, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if t == nil {
		return value, nil
	}
	return t.Decrypt(ctx, keyName, value)
}

// validateKeyName — узкий whitelist на имя ключа, чтобы не давать инъекций
// в Vault path. Соответствует требованиям Vault Transit (alphanum + _ - .).
func validateKeyName(name string) error {
	if name == "" {
		return errors.New("secrets/transit: empty key name")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("secrets/transit: invalid key name %q", name)
		}
	}
	return nil
}

// MockTransit — детерминистический "шифратор" для тестов. Реализует Transit:
//   - Encrypt: XOR-маска по ключу + base64 + префикс "mock:" — стабильно
//     одинаковый ciphertext для одного и того же plaintext+key, что
//     позволяет проверять round-trip и инвариант "шифр != plaintext".
//   - Decrypt: обратная операция.
//   - EnsureKey: записывает ключ во внутренний set; не вызывает сеть.
//
// ВНИМАНИЕ: это НЕ криптография. Использовать ТОЛЬКО в тестах и dev-сценариях.
type MockTransit struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

// NewMockTransit — конструктор.
func NewMockTransit() *MockTransit {
	return &MockTransit{keys: make(map[string]struct{})}
}

const mockPrefix = "mock:v1:"

// Encrypt применяет XOR-маску из keyName к plaintext, возвращает
// base64(masked) с префиксом для отличимости.
func (m *MockTransit) Encrypt(_ context.Context, keyName, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := validateKeyName(keyName); err != nil {
		return "", err
	}
	m.mu.Lock()
	if _, ok := m.keys[keyName]; !ok {
		// EnsureKey должен был быть вызван заранее; для удобства тестов
		// делаем lazy-create — это не нарушает контракт реального клиента,
		// у которого в проде ключ обязан существовать (EnsureKey идемпотентен).
		m.keys[keyName] = struct{}{}
	}
	m.mu.Unlock()

	masked := xorMask([]byte(plaintext), []byte(keyName))
	return mockPrefix + base64.StdEncoding.EncodeToString(masked), nil
}

// Decrypt — обратная XOR-операция.
func (m *MockTransit) Decrypt(_ context.Context, keyName, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if err := validateKeyName(keyName); err != nil {
		return "", err
	}
	if !strings.HasPrefix(ciphertext, mockPrefix) {
		return "", fmt.Errorf("secrets/transit/mock: not a mock ciphertext")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, mockPrefix))
	if err != nil {
		return "", fmt.Errorf("secrets/transit/mock: bad base64: %w", err)
	}
	return string(xorMask(raw, []byte(keyName))), nil
}

// EnsureKey — no-op запоминание ключа для совместимости с Transit.
func (m *MockTransit) EnsureKey(_ context.Context, keyName string) error {
	if err := validateKeyName(keyName); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[keyName] = struct{}{}
	return nil
}

// HasKey — диагностический метод для тестов.
func (m *MockTransit) HasKey(keyName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.keys[keyName]
	return ok
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

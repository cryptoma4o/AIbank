package piiencryption

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aibank/platform/packages/secrets"
)

// vaultMock — минимальный httptest-mock, имитирующий Vault Transit
// /v1/transit/encrypt/<key> и /v1/transit/decrypt/<key>.
type vaultMock struct {
	encryptCalls int32
	decryptCalls int32
	keyCalls     int32

	lastEncryptKey string
	lastDecryptKey string
}

func (vm *vaultMock) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/transit/encrypt/"):
			atomic.AddInt32(&vm.encryptCalls, 1)
			vm.lastEncryptKey = strings.TrimPrefix(r.URL.Path, "/v1/transit/encrypt/")

			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			_ = json.Unmarshal(body, &req)

			// Возвращаем "ciphertext" version 2 — проверим что versioned
			// формат корректно проходит насквозь.
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"request_id": "test-encrypt",
				"data": map[string]interface{}{
					"ciphertext":  "vault:v2:" + base64.StdEncoding.EncodeToString([]byte("encrypted-blob")),
					"key_version": 2,
				},
			})

		case strings.HasPrefix(r.URL.Path, "/v1/transit/decrypt/"):
			atomic.AddInt32(&vm.decryptCalls, 1)
			vm.lastDecryptKey = strings.TrimPrefix(r.URL.Path, "/v1/transit/decrypt/")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"request_id": "test-decrypt",
				"data": map[string]interface{}{
					"plaintext": base64.StdEncoding.EncodeToString([]byte("decrypted-pii")),
				},
			})

		case strings.HasPrefix(r.URL.Path, "/v1/transit/keys/"):
			atomic.AddInt32(&vm.keyCalls, 1)
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	})
}

func newVaultPIIForTest(t *testing.T, addr string) *VaultPIIEncryptor {
	t.Helper()
	transit, err := secrets.NewTransitClient(secrets.TransitConfig{
		Address:        addr,
		Token:          "test-token",
		MountPath:      "transit",
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransitClient: %v", err)
	}

	enc, err := NewVaultPIIEncryptor(VaultPIIConfig{
		Transit:    transit,
		HashSecret: []byte("test-hash-secret-32-bytes-long!!"),
	})
	if err != nil {
		t.Fatalf("NewVaultPIIEncryptor: %v", err)
	}
	return enc
}

func TestVaultPII_Encrypt_HitsCorrectKey(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)

	ct, err := enc.Encrypt(context.Background(), FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, "vault:v2:") {
		t.Fatalf("expected versioned vault: prefix, got %q", ct)
	}
	if vm.encryptCalls != 1 {
		t.Fatalf("expected 1 encrypt call, got %d", vm.encryptCalls)
	}
	if vm.lastEncryptKey != "pii-snils" {
		t.Fatalf("expected key pii-snils, got %q", vm.lastEncryptKey)
	}
}

func TestVaultPII_Encrypt_PerFieldKeys(t *testing.T) {
	// Разные FieldType должны бить в разные Vault Transit endpoint'ы.
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)
	ctx := context.Background()

	cases := []struct {
		field   FieldType
		wantKey string
	}{
		{FieldPassportNumber, "pii-passport-number"},
		{FieldPassportIssuer, "pii-passport-issuer"},
		{FieldSNILS, "pii-snils"},
		{FieldPersonalINN, "pii-personal-inn"},
		{FieldBankAccount, "pii-bank-account"},
		{FieldPhone, "pii-phone"},
	}

	for _, tc := range cases {
		_, err := enc.Encrypt(ctx, tc.field, "test-value")
		if err != nil {
			t.Fatalf("Encrypt(%s): %v", tc.field, err)
		}
		if vm.lastEncryptKey != tc.wantKey {
			t.Errorf("Encrypt(%s) hit key %q, want %q", tc.field, vm.lastEncryptKey, tc.wantKey)
		}
	}
}

func TestVaultPII_Decrypt_HitsCorrectKey(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)

	pt, err := enc.Decrypt(context.Background(), FieldPassportNumber, "vault:v1:abc==")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if pt != "decrypted-pii" {
		t.Fatalf("plaintext mismatch: %q", pt)
	}
	if vm.decryptCalls != 1 {
		t.Fatalf("expected 1 decrypt call, got %d", vm.decryptCalls)
	}
	if vm.lastDecryptKey != "pii-passport-number" {
		t.Fatalf("expected key pii-passport-number, got %q", vm.lastDecryptKey)
	}
}

func TestVaultPII_EmptyDoesNotHitVault(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)
	ctx := context.Background()

	if ct, err := enc.Encrypt(ctx, FieldSNILS, ""); err != nil || ct != "" {
		t.Fatalf("Encrypt empty: ct=%q err=%v", ct, err)
	}
	if pt, err := enc.Decrypt(ctx, FieldSNILS, ""); err != nil || pt != "" {
		t.Fatalf("Decrypt empty: pt=%q err=%v", pt, err)
	}
	if vm.encryptCalls != 0 || vm.decryptCalls != 0 {
		t.Fatalf("empty plaintext не должен лезть в Vault: enc=%d dec=%d",
			vm.encryptCalls, vm.decryptCalls)
	}
}

func TestVaultPII_EnsureKeys_CreatesAllFields(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)

	if err := enc.EnsureKeys(context.Background()); err != nil {
		t.Fatalf("EnsureKeys: %v", err)
	}
	want := int32(len(SupportedFields()))
	if vm.keyCalls != want {
		t.Fatalf("expected %d EnsureKey calls (one per field), got %d", want, vm.keyCalls)
	}
}

func TestVaultPII_UnknownField_Rejected(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)
	ctx := context.Background()

	if _, err := enc.Encrypt(ctx, FieldType("legal-inn"), "1234567890"); err == nil {
		t.Fatalf("legal-inn (юрлицо) не должно шифроваться")
	}
	if _, err := enc.Decrypt(ctx, FieldType(""), "vault:v1:x"); err == nil {
		t.Fatalf("пустой FieldType должен отказывать")
	}
	// Подтверждаем что в Vault не пошли запросы для невалидных полей.
	if vm.encryptCalls != 0 || vm.decryptCalls != 0 {
		t.Fatalf("невалидный FieldType не должен попадать в Vault")
	}
}

func TestVaultPII_NewRequiresTransit(t *testing.T) {
	if _, err := NewVaultPIIEncryptor(VaultPIIConfig{HashSecret: make([]byte, 32)}); err == nil {
		t.Fatalf("expected error when Transit is nil")
	}
}

func TestVaultPII_NewRequiresHashSecretMin32(t *testing.T) {
	transit := secrets.NewMockTransit()
	if _, err := NewVaultPIIEncryptor(VaultPIIConfig{
		Transit:    transit,
		HashSecret: []byte("too-short"),
	}); err == nil {
		t.Fatalf("expected error for short HashSecret")
	}
	if _, err := NewVaultPIIEncryptor(VaultPIIConfig{
		Transit:    transit,
		HashSecret: make([]byte, 32),
	}); err != nil {
		t.Fatalf("32-byte HashSecret должен быть accepted: %v", err)
	}
}

func TestVaultPII_EncryptHash_NoVaultCall(t *testing.T) {
	vm := &vaultMock{}
	srv := httptest.NewServer(vm.handler())
	defer srv.Close()

	enc := newVaultPIIForTest(t, srv.URL)

	h1, err := enc.EncryptHash(context.Background(), FieldSNILS, "123-456-789 01")
	if err != nil {
		t.Fatalf("EncryptHash: %v", err)
	}
	if !strings.HasPrefix(h1, "h1:") {
		t.Fatalf("expected h1: prefix, got %q", h1)
	}
	if vm.encryptCalls != 0 || vm.decryptCalls != 0 {
		t.Fatalf("EncryptHash не должен ходить в Vault: enc=%d dec=%d",
			vm.encryptCalls, vm.decryptCalls)
	}

	// Детерминизм между вызовами одного encryptor'а.
	h2, _ := enc.EncryptHash(context.Background(), FieldSNILS, "123-456-789 01")
	if h1 != h2 {
		t.Fatalf("EncryptHash non-deterministic: %q != %q", h1, h2)
	}
}

func TestVaultPII_EncryptHash_DifferentSecretsGiveDifferentHashes(t *testing.T) {
	transit := secrets.NewMockTransit()

	enc1, err := NewVaultPIIEncryptor(VaultPIIConfig{
		Transit:    transit,
		HashSecret: []byte("secret-A-padding-to-32-bytes!!!!"),
	})
	if err != nil {
		t.Fatalf("enc1: %v", err)
	}
	enc2, err := NewVaultPIIEncryptor(VaultPIIConfig{
		Transit:    transit,
		HashSecret: []byte("secret-B-padding-to-32-bytes!!!!"),
	})
	if err != nil {
		t.Fatalf("enc2: %v", err)
	}

	ctx := context.Background()
	h1, _ := enc1.EncryptHash(ctx, FieldSNILS, "123-456-789 01")
	h2, _ := enc2.EncryptHash(ctx, FieldSNILS, "123-456-789 01")
	if h1 == h2 {
		t.Fatalf("разные HashSecret должны давать разные hash'и")
	}
}

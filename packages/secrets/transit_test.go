package secrets

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
)

// helpers ---------------------------------------------------------------------

func writeTransitEncryptResp(w http.ResponseWriter, ciphertext string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id": "test",
		"data": map[string]interface{}{
			"ciphertext": ciphertext,
			"key_version": 1,
		},
	})
}

func writeTransitDecryptResp(w http.ResponseWriter, plaintextRaw string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id": "test",
		"data": map[string]interface{}{
			"plaintext": base64.StdEncoding.EncodeToString([]byte(plaintextRaw)),
		},
	})
}

func newTransitClientForTest(t *testing.T, addr string) *TransitClient {
	t.Helper()
	c, err := NewTransitClient(TransitConfig{
		Address:        addr,
		Token:          "test-token",
		MountPath:      "transit",
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransitClient: %v", err)
	}
	return c
}

// MockTransit -----------------------------------------------------------------

func TestEncrypt_RoundTrip(t *testing.T) {
	m := NewMockTransit()
	ctx := context.Background()
	key := TenantKeyName("alpha")

	if err := m.EnsureKey(ctx, key); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}

	plain := "1234 567890" // паспорт серия+номер, формат для примера
	ct, err := m.Encrypt(ctx, key, plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ct == plain {
		t.Fatalf("ciphertext равен plaintext — XOR не применился")
	}
	if !strings.HasPrefix(ct, mockPrefix) {
		t.Fatalf("expected mock prefix, got %q", ct)
	}

	got, err := m.Decrypt(ctx, key, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}

	// Детерминизм: тот же plaintext+key → тот же ciphertext.
	ct2, _ := m.Encrypt(ctx, key, plain)
	if ct != ct2 {
		t.Fatalf("mock encrypt не детерминистичен: %q != %q", ct, ct2)
	}
}

func TestEncrypt_EmptyPasses(t *testing.T) {
	m := NewMockTransit()
	ctx := context.Background()
	key := TenantKeyName("alpha")

	ct, err := m.Encrypt(ctx, key, "")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if ct != "" {
		t.Fatalf("empty plaintext должен возвращать empty ciphertext, got %q", ct)
	}

	pt, err := m.Decrypt(ctx, key, "")
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if pt != "" {
		t.Fatalf("empty ciphertext должен возвращать empty plaintext, got %q", pt)
	}
}

func TestEncryptOrEmpty_NilTransitPassthrough(t *testing.T) {
	ctx := context.Background()
	key := TenantKeyName("alpha")

	// nil Transit → plaintext проходит насквозь (dev-режим).
	got, err := EncryptOrEmpty(ctx, nil, key, "snils-12345")
	if err != nil {
		t.Fatalf("EncryptOrEmpty(nil): %v", err)
	}
	if got != "snils-12345" {
		t.Fatalf("nil-passthrough не отработал: got %q", got)
	}

	gotD, err := DecryptOrEmpty(ctx, nil, key, "snils-12345")
	if err != nil {
		t.Fatalf("DecryptOrEmpty(nil): %v", err)
	}
	if gotD != "snils-12345" {
		t.Fatalf("nil-passthrough decrypt не отработал: got %q", gotD)
	}
}

// Vault HTTP API --------------------------------------------------------------

func TestVault_Encrypt_HitsAPI(t *testing.T) {
	var calls int32
	var capturedPath string
	var capturedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		writeTransitEncryptResp(w, "vault:v1:abcdef==")
	}))
	defer srv.Close()

	c := newTransitClientForTest(t, srv.URL)
	keyName := TenantKeyName("alpha")

	ct, err := c.Encrypt(context.Background(), keyName, "1234 567890")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ct != "vault:v1:abcdef==" {
		t.Fatalf("ciphertext mismatch: %q", ct)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	wantPath := "/v1/transit/encrypt/" + keyName
	if capturedPath != wantPath {
		t.Fatalf("path mismatch: got %q want %q", capturedPath, wantPath)
	}
	pt, ok := capturedBody["plaintext"].(string)
	if !ok {
		t.Fatalf("body missing plaintext: %+v", capturedBody)
	}
	raw, err := base64.StdEncoding.DecodeString(pt)
	if err != nil {
		t.Fatalf("plaintext should be base64: %v", err)
	}
	if string(raw) != "1234 567890" {
		t.Fatalf("plaintext payload mismatch: %q", string(raw))
	}
}

func TestVault_Encrypt_EmptyDoesNotHitAPI(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	c := newTransitClientForTest(t, srv.URL)
	got, err := c.Encrypt(context.Background(), TenantKeyName("alpha"), "")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if got != "" {
		t.Fatalf("empty plaintext should not return ciphertext, got %q", got)
	}
	if calls != 0 {
		t.Fatalf("empty plaintext should not call Vault, got %d calls", calls)
	}
}

func TestVault_Decrypt_HitsAPI(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		writeTransitDecryptResp(w, "1234 567890")
	}))
	defer srv.Close()

	c := newTransitClientForTest(t, srv.URL)
	keyName := TenantKeyName("alpha")
	pt, err := c.Decrypt(context.Background(), keyName, "vault:v1:abcdef==")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if pt != "1234 567890" {
		t.Fatalf("plaintext mismatch: %q", pt)
	}
	wantPath := "/v1/transit/decrypt/" + keyName
	if capturedPath != wantPath {
		t.Fatalf("path mismatch: got %q want %q", capturedPath, wantPath)
	}
	if capturedBody["ciphertext"] != "vault:v1:abcdef==" {
		t.Fatalf("body ciphertext mismatch: %+v", capturedBody)
	}
}

func TestEnsureKey_Creates(t *testing.T) {
	var calls int32
	var capturedPath string
	var capturedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTransitClientForTest(t, srv.URL)
	keyName := TenantKeyName("alpha")

	if err := c.EnsureKey(context.Background(), keyName); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	wantPath := "/v1/transit/keys/" + keyName
	if capturedPath != wantPath {
		t.Fatalf("path mismatch: got %q want %q", capturedPath, wantPath)
	}
	if capturedBody["type"] != "aes256-gcm96" {
		t.Fatalf("expected type=aes256-gcm96, got %+v", capturedBody)
	}

	// Повторный вызов в рамках процесса не должен идти в Vault.
	if err := c.EnsureKey(context.Background(), keyName); err != nil {
		t.Fatalf("EnsureKey 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("EnsureKey должен кэшировать; calls=%d", calls)
	}
}

func TestPerTenantKey(t *testing.T) {
	cases := []struct {
		tenantID string
		want     string
	}{
		{"alpha", "aibank-tenant-alpha"},
		{"bank_beta", "aibank-tenant-bank_beta"},
		{"t01", "aibank-tenant-t01"},
	}
	for _, c := range cases {
		got := TenantKeyName(c.tenantID)
		if got != c.want {
			t.Errorf("TenantKeyName(%q) = %q; want %q", c.tenantID, got, c.want)
		}
	}
}

func TestValidateKeyName_RejectsInjection(t *testing.T) {
	bad := []string{
		"",
		"alpha/../etc",
		"alpha key",
		"кир",
		"key?injection",
	}
	m := NewMockTransit()
	ctx := context.Background()
	for _, name := range bad {
		if _, err := m.Encrypt(ctx, name, "x"); err == nil {
			t.Errorf("Encrypt(%q) accepted invalid key name", name)
		}
	}
}

func TestNewTransitClient_RequiresAddrAndToken(t *testing.T) {
	if _, err := NewTransitClient(TransitConfig{}); err == nil {
		t.Fatal("expected error for empty config")
	}
	if _, err := NewTransitClient(TransitConfig{Address: "http://x"}); err == nil {
		t.Fatal("expected error for missing token")
	}
	if _, err := NewTransitClient(TransitConfig{Token: "t"}); err == nil {
		t.Fatal("expected error for missing addr")
	}
}

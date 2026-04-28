package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// helper: write a KV v2 envelope { "data": { "data": payload } }
func writeKVv2(w http.ResponseWriter, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id": "test",
		"data": map[string]interface{}{
			"data":     payload,
			"metadata": map[string]interface{}{"version": 1},
		},
	})
}

func TestEnvProvider_Get(t *testing.T) {
	p := NewEnvProvider("")
	t.Setenv("DATABASE_URL", "postgres://x")

	got, err := p.GetSecret(context.Background(), "database/url")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "postgres://x" {
		t.Fatalf("got %q", got)
	}

	if _, err := p.GetSecret(context.Background(), "nope/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	pp := NewEnvProvider("IDENT_")
	t.Setenv("IDENT_JWT_SECRET", "shh")
	got, err = pp.GetSecret(context.Background(), "jwt/secret")
	if err != nil || got != "shh" {
		t.Fatalf("prefix lookup failed: %q err=%v", got, err)
	}
}

func TestVaultProvider_NotConfigured_Errors(t *testing.T) {
	if _, err := NewVaultProvider(VaultConfig{}); err == nil {
		t.Fatal("expected error for empty config")
	}
	if _, err := NewVaultProvider(VaultConfig{Address: "http://x"}); err == nil {
		t.Fatal("expected error for missing token")
	}
	if _, err := NewVaultProvider(VaultConfig{Token: "t"}); err == nil {
		t.Fatal("expected error for missing addr")
	}
}

func TestVaultProvider_Cache(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeKVv2(w, map[string]interface{}{"url": "postgres://from-vault"})
	}))
	defer srv.Close()

	p, err := NewVaultProvider(VaultConfig{
		Address:    srv.URL,
		Token:      "test-token",
		MountPath:  "secret",
		PathPrefix: "aibank/identity-service",
		CacheTTL:   1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}

	for i := 0; i < 3; i++ {
		got, err := p.GetSecret(context.Background(), "database/url")
		if err != nil {
			t.Fatalf("get #%d: %v", i, err)
		}
		if got != "postgres://from-vault" {
			t.Fatalf("got %q", got)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 vault call (cache), got %d", n)
	}

	// Invalidate -> next call hits Vault again.
	p.InvalidateCache()
	if _, err := p.GetSecret(context.Background(), "database/url"); err != nil {
		t.Fatalf("post-invalidate: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("expected 2 calls after invalidate, got %d", n)
	}
}

func TestVaultProvider_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	p, _ := NewVaultProvider(VaultConfig{Address: srv.URL, Token: "t", PathPrefix: "aibank/x"})
	if _, err := p.GetSecret(context.Background(), "missing/key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestChainedProvider_Fallback(t *testing.T) {
	// Vault returns 404 -> ErrNotFound -> chain falls through to env.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	vault, err := NewVaultProvider(VaultConfig{Address: srv.URL, Token: "t", PathPrefix: "aibank/x"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET", "fallback-value")
	chain := NewChainedProvider(vault, NewEnvProvider(""))

	got, err := chain.GetSecret(context.Background(), "jwt/secret")
	if err != nil {
		t.Fatalf("chain get: %v", err)
	}
	if got != "fallback-value" {
		t.Fatalf("expected fallback, got %q", got)
	}

	// Both miss -> ErrNotFound.
	if _, err := chain.GetSecret(context.Background(), "no/such"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestChainedProvider_HardErrorShortCircuits(t *testing.T) {
	// Vault 500 must NOT fall back — returning random env in prod when Vault
	// is broken is exactly the failure mode we want to avoid.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	vault, _ := NewVaultProvider(VaultConfig{Address: srv.URL, Token: "t", PathPrefix: "aibank/x"})
	t.Setenv("JWT_SECRET", "leaked")
	chain := NewChainedProvider(vault, NewEnvProvider(""))
	if _, err := chain.GetSecret(context.Background(), "jwt/secret"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected hard error, got %v", err)
	}
}

func TestVaultProvider_ConnectError(t *testing.T) {
	// Use a closed listener address so Dial fails quickly within timeout.
	p, err := NewVaultProvider(VaultConfig{
		Address:        "http://127.0.0.1:1", // privileged + unbound -> connection refused
		Token:          "t",
		PathPrefix:     "aibank/x",
		RequestTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = p.GetSecret(context.Background(), "database/url")
	if err == nil {
		t.Fatal("expected connect error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("connect error must not collapse to ErrNotFound: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("error took too long (%s) — RequestTimeout not honoured", elapsed)
	}
}

func TestSplitKey(t *testing.T) {
	cases := []struct{ in, path, field string }{
		{"database/url", "database", "url"},
		{"a/b/c", "a/b", "c"},
		{"flat", "flat", "value"},
		{"/leading/slash", "leading", "slash"},
	}
	for _, c := range cases {
		p, f := splitKey(c.in)
		if p != c.path || f != c.field {
			t.Errorf("splitKey(%q) = (%q,%q); want (%q,%q)", c.in, p, f, c.path, c.field)
		}
	}
}

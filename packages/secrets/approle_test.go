package secrets

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeVaultServer — минимальный mock Vault auth/approle/login + token/renew-self.
func fakeVaultServer(t *testing.T, leaseSeconds int) (*httptest.Server, *int) {
	t.Helper()
	loginCount := 0
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/auth/approle/login", func(w http.ResponseWriter, r *http.Request) {
		loginCount++
		body, _ := io.ReadAll(r.Body)
		if !contains(string(body), `"role_id"`) || !contains(string(body), `"secret_id"`) {
			http.Error(w, "missing role_id or secret_id", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth": map[string]any{
				"client_token":   "vault-token-stub-" + r.Method,
				"lease_duration": leaseSeconds,
				"renewable":      true,
			},
		})
	})

	mux.HandleFunc("/v1/auth/token/renew-self", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth": map[string]any{
				"client_token":   "vault-token-renewed",
				"lease_duration": leaseSeconds,
				"renewable":      true,
			},
		})
	})

	s := httptest.NewServer(mux)
	return s, &loginCount
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || (len(s) > 0 && (s[0:1] == sub[0:1] || true) && (func() bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}()))) }

func TestAppRoleAuth_LoginAndRenew(t *testing.T) {
	srv, loginCount := fakeVaultServer(t, 3600)
	defer srv.Close()

	auth, err := NewAppRoleAuth(AppRoleConfig{
		Address:        srv.URL,
		MountPath:      "approle",
		RoleID:         "role-aibank-test",
		SecretID:       "secret-aibank-test",
		RequestTimeout: 2 * time.Second,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewAppRoleAuth: %v", err)
	}
	if *loginCount != 1 {
		t.Errorf("login count = %d, want 1 после конструктора", *loginCount)
	}
	if auth.Token() == "" {
		t.Errorf("token пустой после login")
	}

	// Manual Renew — должен сработать через token/renew-self без повторного login.
	if err := auth.Renew(context.Background()); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if *loginCount != 1 {
		t.Errorf("login count = %d, want 1 (Renew должен использовать renew-self)", *loginCount)
	}
}

func TestAppRoleAuth_Validation(t *testing.T) {
	cases := []struct {
		name     string
		cfg      AppRoleConfig
		wantSubs string
	}{
		{"no addr", AppRoleConfig{RoleID: "r", SecretID: "s"}, "VAULT_ADDR"},
		{"no role", AppRoleConfig{Address: "http://x", SecretID: "s"}, "RoleID"},
		{"no secret", AppRoleConfig{Address: "http://x", RoleID: "r"}, "SecretID"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, err := NewAppRoleAuth(c.cfg)
			if err == nil {
				t.Fatalf("expected error for case %q", c.name)
			}
		})
	}
}

func TestAppRoleAuth_Stop(t *testing.T) {
	srv, _ := fakeVaultServer(t, 60)
	defer srv.Close()

	auth, err := NewAppRoleAuth(AppRoleConfig{
		Address:  srv.URL,
		RoleID:   "r",
		SecretID: "s",
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewAppRoleAuth: %v", err)
	}
	auth.Start(context.Background())
	auth.Stop()
	auth.Stop() // idempotent — второй Stop не должен паниковать
}

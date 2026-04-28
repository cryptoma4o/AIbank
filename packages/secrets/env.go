package secrets

import (
	"context"
	"os"
	"strings"
)

// EnvProvider reads secrets from process environment variables.
//
// It is the dev-mode fallback (see docs/security-architecture.md § 7.3:
// "Никогда в env-переменных production-сервисов" — production must use Vault).
//
// Key translation: a path-like key ("database/url") is normalised to an env
// var name by replacing '/' and '-' with '_' and uppercasing — so
// "database/url" -> "DATABASE_URL", "jwt/secret" -> "JWT_SECRET". An optional
// prefix can be supplied to namespace per-service (e.g. "TENANT_").
type EnvProvider struct {
	prefix string
}

// NewEnvProvider builds an EnvProvider. prefix may be empty.
func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{prefix: prefix}
}

// Name implements Provider.
func (e *EnvProvider) Name() string { return "env" }

// GetSecret implements Provider. Returns ErrNotFound when env var is unset
// or empty — callers can chain via ChainedProvider.
func (e *EnvProvider) GetSecret(_ context.Context, key string) (string, error) {
	envName := e.prefix + envName(key)
	if v, ok := os.LookupEnv(envName); ok && v != "" {
		return v, nil
	}
	return "", ErrNotFound
}

// envName normalises a backend-agnostic key into an env var name.
func envName(key string) string {
	r := strings.NewReplacer("/", "_", "-", "_", ".", "_")
	return strings.ToUpper(r.Replace(key))
}

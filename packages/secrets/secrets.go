// Package secrets provides a small, stable abstraction over secret backends
// (HashiCorp Vault KV v2, environment variables, chained fallbacks).
//
// Per docs/security-architecture.md § 7.3 production services MUST NOT read
// secrets from process env vars; they go through Vault (sidecar/CSI). In dev
// we transparently fall back to env to keep onboarding cheap.
//
// Wiring lives in factory.go via BuildProvider().
package secrets

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a secret cannot be located in any backend.
// Callers should treat this as a configuration error in production.
var ErrNotFound = errors.New("secrets: not found")

// Provider is the minimal contract every secret backend implements.
//
// key is backend-specific. For Vault it is the relative path under the
// service mount root (e.g. "database/url"). For env it is the env var name
// after backend-specific transformation (see EnvProvider).
type Provider interface {
	// GetSecret returns the secret value for key or ErrNotFound. Implementations
	// MUST honour ctx cancellation and timeouts.
	GetSecret(ctx context.Context, key string) (string, error)
	// Name returns a short identifier used in logs/errors (e.g. "vault", "env").
	Name() string
}

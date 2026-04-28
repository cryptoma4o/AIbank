package secrets

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// FactoryConfig configures BuildProvider. Empty values mean "read from env".
type FactoryConfig struct {
	// Backend selects the strategy: "vault", "env", or "chained" (default).
	// Empty defaults to "chained" — Vault first if configured, env fallback.
	Backend string
	// ServiceName is used as the per-service Vault path prefix:
	// "aibank/<service-name>".  Required for backend=vault|chained.
	ServiceName string
	// EnvPrefix is prepended to env var names (e.g. "IDENTITY_").
	EnvPrefix string
	// CacheTTL overrides Vault cache TTL (default 5 min).
	CacheTTL time.Duration
}

// BuildProvider constructs a Provider from cfg + env. ENV variables read:
//
//	SECRETS_BACKEND     vault | env | chained (default chained)
//	VAULT_ADDR          Vault server URL
//	VAULT_TOKEN         Vault token (TODO: AppRole / Vault Agent)
//	VAULT_NAMESPACE     Optional Enterprise namespace
//	VAULT_MOUNT         KV v2 mount path (default "secret")
//	VAULT_PATH_PREFIX   Override per-service path prefix
//
// Behaviour:
//   - backend=env       -> EnvProvider only.
//   - backend=vault     -> VaultProvider only; misconfig is a hard error.
//   - backend=chained   -> Vault if VAULT_ADDR+VAULT_TOKEN set, else just Env.
func BuildProvider(cfg FactoryConfig) (Provider, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.Backend))
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(os.Getenv("SECRETS_BACKEND")))
	}
	if backend == "" {
		backend = "chained"
	}

	envProv := NewEnvProvider(cfg.EnvPrefix)

	switch backend {
	case "env":
		return envProv, nil
	case "vault":
		vp, err := buildVaultFromEnv(cfg)
		if err != nil {
			return nil, err
		}
		return vp, nil
	case "chained":
		if !vaultEnvConfigured() {
			return envProv, nil
		}
		vp, err := buildVaultFromEnv(cfg)
		if err != nil {
			return nil, err
		}
		return NewChainedProvider(vp, envProv), nil
	default:
		return nil, fmt.Errorf("secrets: unknown SECRETS_BACKEND=%q", backend)
	}
}

func vaultEnvConfigured() bool {
	return os.Getenv("VAULT_ADDR") != "" && os.Getenv("VAULT_TOKEN") != ""
}

func buildVaultFromEnv(cfg FactoryConfig) (*VaultProvider, error) {
	prefix := os.Getenv("VAULT_PATH_PREFIX")
	if prefix == "" && cfg.ServiceName != "" {
		prefix = "aibank/" + cfg.ServiceName
	}
	mount := os.Getenv("VAULT_MOUNT")
	if mount == "" {
		mount = "secret"
	}
	return NewVaultProvider(VaultConfig{
		Address:        os.Getenv("VAULT_ADDR"),
		Token:          os.Getenv("VAULT_TOKEN"),
		Namespace:      os.Getenv("VAULT_NAMESPACE"),
		MountPath:      mount,
		PathPrefix:     prefix,
		CacheTTL:       cfg.CacheTTL,
		RequestTimeout: 5 * time.Second,
	})
}

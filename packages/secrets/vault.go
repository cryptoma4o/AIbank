package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

// VaultConfig configures a VaultProvider.
//
// All fields except Address+Token (or AppRole creds, future work) are
// optional. See README for the full ENV-var contract.
type VaultConfig struct {
	// Address is the Vault server URL (VAULT_ADDR).
	Address string
	// Token is the Vault token (VAULT_TOKEN). For prod use AppRole / Vault
	// Agent — left as TODO in README.
	Token string
	// Namespace optionally scopes all reads (Vault Enterprise; OSS ignores).
	Namespace string
	// MountPath is the KV v2 mount, default "secret".
	MountPath string
	// PathPrefix is the per-service prefix under the mount, e.g.
	// "aibank/identity-service".  Final read path is
	//   <mount>/data/<prefix>/<key>
	PathPrefix string
	// CacheTTL controls in-memory caching (default 5 min). Zero disables
	// caching.
	CacheTTL time.Duration
	// RequestTimeout is the per-call deadline (default 5 sec).
	RequestTimeout time.Duration
}

// VaultProvider reads KV v2 secrets from HashiCorp Vault with a small
// in-memory cache to avoid hammering Vault on every request.
//
// Cache strategy: per-key value + expiry timestamp; on hit and not expired,
// no network call is made. Cache is process-local — rotation requires a
// service restart or a future SIGHUP/reload (TODO).
type VaultProvider struct {
	cfg    VaultConfig
	client *vaultapi.Client

	mu    sync.RWMutex
	cache map[string]vaultEntry
	now   func() time.Time // overridable for tests
}

type vaultEntry struct {
	value   string
	expires time.Time
}

// NewVaultProvider validates cfg, builds a Vault API client and returns a
// ready-to-use provider. It does NOT pre-fetch secrets; failures surface on
// first GetSecret.
func NewVaultProvider(cfg VaultConfig) (*VaultProvider, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("secrets/vault: VAULT_ADDR is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("secrets/vault: VAULT_TOKEN is required (AppRole TODO)")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "secret"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Second
	}

	apiCfg := vaultapi.DefaultConfig()
	apiCfg.Address = cfg.Address
	apiCfg.Timeout = cfg.RequestTimeout
	client, err := vaultapi.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("secrets/vault: build client: %w", err)
	}
	client.SetToken(cfg.Token)
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	return &VaultProvider{
		cfg:    cfg,
		client: client,
		cache:  make(map[string]vaultEntry),
		now:    time.Now,
	}, nil
}

// Name implements Provider.
func (v *VaultProvider) Name() string { return "vault" }

// GetSecret implements Provider with KV v2 semantics.
//
// The KV v2 API returns a JSON envelope:
//
//	{ "data": { "data": { "<field>": "<value>" }, "metadata": {...} } }
//
// We treat the last path segment as the field name and everything before
// as the secret path. Examples (with PathPrefix="aibank/identity-service"):
//
//	key="database/url"     -> path="aibank/identity-service/database", field="url"
//	key="jwt/secret"       -> path="aibank/identity-service/jwt",      field="secret"
//	key="single"           -> path="aibank/identity-service/single",   field="value"
func (v *VaultProvider) GetSecret(ctx context.Context, key string) (string, error) {
	if cached, ok := v.lookup(key); ok {
		return cached, nil
	}

	path, field := splitKey(key)
	fullPath := v.cfg.MountPath + "/data/"
	if v.cfg.PathPrefix != "" {
		fullPath += strings.Trim(v.cfg.PathPrefix, "/") + "/"
	}
	fullPath += path

	cctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	secret, err := v.client.Logical().ReadWithContext(cctx, fullPath)
	if err != nil {
		return "", fmt.Errorf("secrets/vault: read %q: %w", fullPath, err)
	}
	if secret == nil || secret.Data == nil {
		return "", ErrNotFound
	}

	// KV v2 wraps user data under .data.data
	dataBlock, ok := secret.Data["data"].(map[string]interface{})
	if !ok || dataBlock == nil {
		return "", ErrNotFound
	}
	raw, ok := dataBlock[field]
	if !ok {
		return "", ErrNotFound
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("secrets/vault: field %q at %q is not a string", field, fullPath)
	}
	v.store(key, str)
	return str, nil
}

func (v *VaultProvider) lookup(key string) (string, bool) {
	if v.cfg.CacheTTL <= 0 {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	e, ok := v.cache[key]
	if !ok {
		return "", false
	}
	if v.now().After(e.expires) {
		return "", false
	}
	return e.value, true
}

func (v *VaultProvider) store(key, value string) {
	if v.cfg.CacheTTL <= 0 {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cache[key] = vaultEntry{value: value, expires: v.now().Add(v.cfg.CacheTTL)}
}

// InvalidateCache wipes the in-memory cache. Useful for rotation hooks.
func (v *VaultProvider) InvalidateCache() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cache = make(map[string]vaultEntry)
}

// splitKey splits "a/b/c" into ("a/b", "c"). Single-segment keys fall back
// to field "value" so callers can store flat secrets.
func splitKey(key string) (path, field string) {
	key = strings.Trim(key, "/")
	idx := strings.LastIndex(key, "/")
	if idx < 0 {
		return key, "value"
	}
	return key[:idx], key[idx+1:]
}

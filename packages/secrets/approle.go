package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

// AppRoleConfig — параметры AppRole login flow (ADR-0013).
//
// AppRole — это правильный production-режим Vault auth: каждый сервис
// получает per-pod ephemeral token с автообновлением, в отличие от
// статического VAULT_TOKEN.
type AppRoleConfig struct {
	// Address — VAULT_ADDR.
	Address string
	// Namespace — Vault Enterprise namespace (опционально).
	Namespace string
	// MountPath — путь монтирования AppRole auth ("approle" по умолчанию).
	MountPath string
	// RoleID — публичный идентификатор роли (можно держать в ConfigMap).
	RoleID string
	// SecretID — секретный part-of-credentials (хранится в k8s Secret или
	// приходит через Vault Agent / CSI).
	SecretID string
	// SecretPath / SecretKVMount — параметры VaultConfig для последующего
	// VaultProvider, который будет использовать полученный token.
	SecretKVMount  string
	SecretPath     string
	CacheTTL       time.Duration
	RequestTimeout time.Duration

	// RenewMargin — за сколько до expiration token будет обновлён
	// (default 30% от leaseDuration).
	RenewMargin float64

	// Logger — для diagnostic events.
	Logger *slog.Logger
}

// AppRoleAuth — менеджер lifecycle AppRole token: login + auto-renew.
//
// Не дублирует VaultProvider, а интегрируется через NewVaultProviderFromAppRole:
// AppRoleAuth логинится → возвращает live VaultProvider, который читает
// secrets с auto-renewed token'ом.
type AppRoleAuth struct {
	cfg    AppRoleConfig
	client *vaultapi.Client
	log    *slog.Logger

	mu     sync.RWMutex
	token  string
	expiry time.Time

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewAppRoleAuth выполняет первый login и возвращает менеджер.
// Background goroutine стартует только после Start().
func NewAppRoleAuth(cfg AppRoleConfig) (*AppRoleAuth, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, errors.New("secrets/approle: VAULT_ADDR is required")
	}
	if strings.TrimSpace(cfg.RoleID) == "" {
		return nil, errors.New("secrets/approle: RoleID is required")
	}
	if strings.TrimSpace(cfg.SecretID) == "" {
		return nil, errors.New("secrets/approle: SecretID is required")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "approle"
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Second
	}
	if cfg.RenewMargin <= 0 || cfg.RenewMargin >= 1 {
		cfg.RenewMargin = 0.30 // обновлять при 70% жизни
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	apiCfg := vaultapi.DefaultConfig()
	apiCfg.Address = cfg.Address
	apiCfg.Timeout = cfg.RequestTimeout
	client, err := vaultapi.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("secrets/approle: build client: %w", err)
	}
	if cfg.Namespace != "" {
		client.SetNamespace(cfg.Namespace)
	}

	a := &AppRoleAuth{
		cfg:    cfg,
		client: client,
		log:    logger,
		stopCh: make(chan struct{}),
	}
	if err := a.login(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

// Token возвращает текущий valid token. Может быть пустым только до login()
// (что нереально, потому что NewAppRoleAuth требует первого login'а).
func (a *AppRoleAuth) Token() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token
}

// Start запускает background goroutine для auto-renewal. Без Start
// токен будет работать до expiry (типично 1h), потом нужно вручную
// вызывать Renew(). Stop() корректно завершает renewer.
func (a *AppRoleAuth) Start(ctx context.Context) {
	go a.renewLoop(ctx)
}

// Stop сигналит renewer выйти. Idempotent.
func (a *AppRoleAuth) Stop() {
	a.stopOnce.Do(func() { close(a.stopCh) })
}

// VaultProvider возвращает VaultProvider, использующий live AppRole token.
// При каждом GetSecret клиент отправляет текущий token (потокобезопасно).
//
// На token-rotation VaultProvider автоматически использует новый token —
// AppRoleAuth обновляет client.SetToken() в renewLoop.
func (a *AppRoleAuth) VaultProvider() (*VaultProvider, error) {
	cfg := VaultConfig{
		Address:        a.cfg.Address,
		Namespace:      a.cfg.Namespace,
		MountPath:      a.cfg.SecretKVMount,
		PathPrefix:     a.cfg.SecretPath,
		CacheTTL:       a.cfg.CacheTTL,
		RequestTimeout: a.cfg.RequestTimeout,
		// Token проставляется через client.SetToken вручную.
		Token: a.Token(),
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "secret"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	return &VaultProvider{
		cfg:    cfg,
		client: a.client, // shared client — token обновляется AppRoleAuth'ом
		cache:  make(map[string]vaultEntry),
		now:    time.Now,
	}, nil
}

func (a *AppRoleAuth) login(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, a.cfg.RequestTimeout)
	defer cancel()

	path := fmt.Sprintf("auth/%s/login", strings.Trim(a.cfg.MountPath, "/"))
	resp, err := a.client.Logical().WriteWithContext(cctx, path, map[string]interface{}{
		"role_id":   a.cfg.RoleID,
		"secret_id": a.cfg.SecretID,
	})
	if err != nil {
		return fmt.Errorf("secrets/approle: login: %w", err)
	}
	if resp == nil || resp.Auth == nil {
		return errors.New("secrets/approle: login: empty auth response")
	}
	token := resp.Auth.ClientToken
	leaseDur := time.Duration(resp.Auth.LeaseDuration) * time.Second
	if leaseDur <= 0 {
		leaseDur = 1 * time.Hour // fallback
	}

	a.mu.Lock()
	a.token = token
	a.expiry = time.Now().Add(leaseDur)
	a.mu.Unlock()

	a.client.SetToken(token)
	a.log.Info("vault approle login OK",
		"lease_duration", leaseDur,
		"renewable", resp.Auth.Renewable)
	return nil
}

// Renew пытается продлить текущий token. Если Vault отдаёт renewable=false
// или истёк — выполняется полный re-login.
func (a *AppRoleAuth) Renew(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, a.cfg.RequestTimeout)
	defer cancel()

	resp, err := a.client.Auth().Token().RenewSelfWithContext(cctx, 0)
	if err != nil || resp == nil || resp.Auth == nil {
		// Renewal failed — fallback to re-login (типично если token уже истёк).
		a.log.Warn("vault token renew failed, re-login", "err", err)
		return a.login(ctx)
	}
	leaseDur := time.Duration(resp.Auth.LeaseDuration) * time.Second
	if leaseDur <= 0 {
		leaseDur = 1 * time.Hour
	}
	a.mu.Lock()
	a.expiry = time.Now().Add(leaseDur)
	a.mu.Unlock()
	a.log.Debug("vault token renewed", "lease_duration", leaseDur)
	return nil
}

// renewLoop спит до 70% жизни token'а, потом Renew. На ошибки — backoff
// 30 секунд и повтор. Завершается на Stop() или ctx.Done().
func (a *AppRoleAuth) renewLoop(ctx context.Context) {
	for {
		a.mu.RLock()
		expiry := a.expiry
		a.mu.RUnlock()

		left := time.Until(expiry)
		if left <= 0 {
			left = 1 * time.Second
		}
		// Renew за RenewMargin до expiry (по умолчанию 30%).
		wait := time.Duration(float64(left) * (1 - a.cfg.RenewMargin))
		if wait < 30*time.Second {
			wait = 30 * time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-time.After(wait):
		}

		if err := a.Renew(ctx); err != nil {
			a.log.Error("vault renew failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			case <-time.After(30 * time.Second):
				continue
			}
		}
	}
}

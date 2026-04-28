package secrets

import (
	"context"
	"errors"
	"fmt"
)

// ChainedProvider tries each provider in order until one returns a value.
// It distinguishes "not found, try next" (ErrNotFound) from real errors
// (network/auth) — those are returned immediately and break the chain so
// we never silently fall back to env in prod when Vault is misconfigured.
type ChainedProvider struct {
	providers []Provider
}

// NewChainedProvider wires providers in priority order.
func NewChainedProvider(providers ...Provider) *ChainedProvider {
	return &ChainedProvider{providers: providers}
}

// Name implements Provider.
func (c *ChainedProvider) Name() string {
	names := make([]string, 0, len(c.providers))
	for _, p := range c.providers {
		names = append(names, p.Name())
	}
	return fmt.Sprintf("chained(%v)", names)
}

// GetSecret implements Provider. ErrNotFound from a provider triggers the
// next; any other error short-circuits.
func (c *ChainedProvider) GetSecret(ctx context.Context, key string) (string, error) {
	if len(c.providers) == 0 {
		return "", ErrNotFound
	}
	var lastErr error
	for _, p := range c.providers {
		val, err := p.GetSecret(ctx, key)
		if err == nil {
			return val, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("secrets/chained: %s: %w", p.Name(), err)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNotFound
	}
	return "", lastErr
}

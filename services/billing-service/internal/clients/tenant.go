package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aibank/billing-service/internal/domain"
)

// TenantClient — минимальный HTTP-клиент к tenant-service для резолва pricing_tier.
type TenantClient struct {
	baseURL string
	http    *http.Client
	log     *slog.Logger
}

func NewTenantClient(baseURL string, log *slog.Logger) *TenantClient {
	return &TenantClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
		log:     log,
	}
}

// tenantConfigEnvelope — то, что отдаёт tenant-service по GET /v1/tenants/{id}/config.
// raw_config — base64-bytes (JSON-marshalling), внутри — сериализованный YAML/JSON.
// Для MVP мы парсим raw_config как JSON и достаём contract.pricing_tier.
type tenantContractView struct {
	Contract struct {
		PricingTier string `json:"pricing_tier"`
	} `json:"contract"`
}

// ResolveTier возвращает Tier для тенанта. На любую ошибку (сеть, отсутствие
// поля, не-JSON config) — пишет warn и возвращает domain.TierBasic.
//
// Для MVP мы пробуем GET /v1/tenants/{id}; если у нас нет endpoint'а с tier'ом,
// возвращаем basic как safe default per ADR-0010 § 6 «Ad-hoc корректировки».
func (c *TenantClient) ResolveTier(ctx context.Context, tenantID string) domain.Tier {
	tier, err := c.fetchTier(ctx, tenantID)
	if err != nil {
		if c.log != nil {
			c.log.Warn("не удалось получить tier тенанта — fallback на basic",
				"tenant_id", tenantID, "err", err)
		}
		return domain.TierBasic
	}
	return tier
}

func (c *TenantClient) fetchTier(ctx context.Context, tenantID string) (domain.Tier, error) {
	if c.baseURL == "" {
		return domain.TierBasic, errors.New("tenant-service URL not configured")
	}
	url := fmt.Sprintf("%s/v1/tenants/%s/config", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain.TierBasic, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tenant-service returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	tier := extractTierFromConfig(body)
	if tier == "" {
		return domain.TierBasic, nil
	}
	return normalizeTier(tier), nil
}

// extractTierFromConfig вытаскивает contract.pricing_tier из ответа tenant-service.
// Поддерживает два формата envelope:
//
//	{ "raw_config": "<bytes>", ... }     // JSON-кодированный []byte
//	{ "contract": { "pricing_tier": ... } } // прямой view
func extractTierFromConfig(body []byte) string {
	// Variant 1: прямой view.
	var direct tenantContractView
	if err := json.Unmarshal(body, &direct); err == nil && direct.Contract.PricingTier != "" {
		return direct.Contract.PricingTier
	}

	// Variant 2: tenant-service возвращает {raw_config: base64-bytes, ...}
	// — тогда raw_config внутри уже JSON или YAML с полем contract.pricing_tier.
	var envelope struct {
		RawConfig []byte `json:"raw_config"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.RawConfig) > 0 {
		var nested tenantContractView
		if err := json.Unmarshal(envelope.RawConfig, &nested); err == nil {
			return nested.Contract.PricingTier
		}
	}
	return ""
}

func normalizeTier(raw string) domain.Tier {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(domain.TierPremium):
		return domain.TierPremium
	case string(domain.TierEnterprise):
		return domain.TierEnterprise
	default:
		return domain.TierBasic
	}
}

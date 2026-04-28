package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Tenant — payload tenant-service'а.
type Tenant struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BIK            string `json:"bik"`
	INN            string `json:"inn"`
	Status         string `json:"status"`
	DeploymentMode string `json:"deployment_mode"`
}

// TenantConfig — payload tenant-service /config.
type TenantConfig struct {
	TenantID      string    `json:"tenant_id"`
	RawConfig     string    `json:"raw_config"`
	SchemaVersion string    `json:"schema_version"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TenantClient — клиент к tenant-service.
type TenantClient struct {
	baseURL string
	tr      *transport
}

func NewTenantClient(baseURL string) *TenantClient {
	return &TenantClient{baseURL: baseURL, tr: newTransport()}
}

// Get — GET /v1/tenants/{id}.
func (c *TenantClient) Get(ctx context.Context, id string) (*Tenant, error) {
	url := fmt.Sprintf("%s/v1/tenants/%s", c.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var p Tenant
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetConfig — GET /v1/tenants/{id}/config.
func (c *TenantClient) GetConfig(ctx context.Context, id string) (*TenantConfig, error) {
	url := fmt.Sprintf("%s/v1/tenants/%s/config", c.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	// Парсим в обёртку, потому что сервер хранит raw_config как []byte.
	var raw struct {
		TenantID      string          `json:"tenant_id"`
		RawConfig     json.RawMessage `json:"raw_config"`
		SchemaVersion string          `json:"schema_version"`
		UpdatedAt     time.Time       `json:"updated_at"`
	}
	if err := c.tr.doJSON(req, &raw); err != nil {
		return nil, err
	}
	return &TenantConfig{
		TenantID:      raw.TenantID,
		RawConfig:     string(raw.RawConfig),
		SchemaVersion: raw.SchemaVersion,
		UpdatedAt:     raw.UpdatedAt,
	}, nil
}

// Suspend — PUT /v1/tenants/{id} с обновлением статуса; обёртка вокруг
// изменения статуса тенанта.
//
// TODO: сегодня tenant-service не выставляет dedicated suspend endpoint;
// здесь — конструктивный stub.  Финальная реализация пойдёт в одном PR
// с tenant-service.
func (c *TenantClient) Suspend(ctx context.Context, id, reason string) (*Tenant, error) {
	body := map[string]any{
		"status":             "suspended",
		"suspension_reason":  reason,
	}
	url := fmt.Sprintf("%s/v1/tenants/%s/suspend", c.baseURL, id)
	req, err := newJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	var p Tenant
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

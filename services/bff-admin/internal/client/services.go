package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AdminClients struct {
	TenantURL  string
	AuditURL   string
	RiskURL    string
	ClientURL  string
	UBOURL     string
	httpClient *http.Client
}

func NewAdminClients(tenantURL, auditURL, riskURL, clientURL, uboURL string) *AdminClients {
	return &AdminClients{
		TenantURL:  tenantURL,
		AuditURL:   auditURL,
		RiskURL:    riskURL,
		ClientURL:  clientURL,
		UBOURL:     uboURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *AdminClients) GetTenant(ctx context.Context, id string) (map[string]any, error) {
	return c.get(ctx, c.TenantURL+"/v1/tenants/"+id)
}

func (c *AdminClients) ListAuditEvents(ctx context.Context, tenantID string, limit int) ([]any, error) {
	url := fmt.Sprintf("%s/v1/events?tenant_id=%s&limit=%d", c.AuditURL, tenantID, limit)
	resp, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	if events, ok := resp["events"].([]any); ok {
		return events, nil
	}
	return []any{}, nil
}

func (c *AdminClients) GetClient(ctx context.Context, tenantID, clientID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ClientURL+"/v1/clients/"+clientID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tenant-ID", tenantID)
	return c.doJSON(req)
}

func (c *AdminClients) GetUBOGraph(ctx context.Context, tenantID, appID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.UBOURL+"/v1/graphs/"+appID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tenant-ID", tenantID)
	return c.doJSON(req)
}

func (c *AdminClients) get(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

func (c *AdminClients) doJSON(req *http.Request) (map[string]any, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bff-admin: %w", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("bff-admin: decode: %w", err)
	}
	return result, nil
}

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ServiceClients struct {
	OnboardingURL string
	ClientURL     string
	DocumentURL   string
	IdentityURL   string
	httpClient    *http.Client
}

func NewServiceClients(onboardingURL, clientURL, documentURL, identityURL string) *ServiceClients {
	return &ServiceClients{
		OnboardingURL: onboardingURL,
		ClientURL:     clientURL,
		DocumentURL:   documentURL,
		IdentityURL:   identityURL,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ServiceClients) GetApplication(ctx context.Context, id string) (map[string]any, error) {
	return c.get(ctx, c.OnboardingURL+"/v1/applications/"+id)
}

func (c *ServiceClients) GetClient(ctx context.Context, tenantID, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ClientURL+"/v1/clients/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tenant-ID", tenantID)
	return c.doJSON(req)
}

func (c *ServiceClients) GetClientEvents(ctx context.Context, tenantID, clientID string) ([]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ClientURL+"/v1/clients/"+clientID+"/events?limit=20", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tenant-ID", tenantID)
	resp, err := c.doJSON(req)
	if err != nil {
		return nil, err
	}
	if events, ok := resp["events"].([]any); ok {
		return events, nil
	}
	return []any{}, nil
}

func (c *ServiceClients) get(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

func (c *ServiceClients) doJSON(req *http.Request) (map[string]any, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bff: request failed: %w", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("bff: decode: %w", err)
	}
	return result, nil
}

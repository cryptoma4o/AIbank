// Package client implements an HTTP client to the tenant-service public API.
//
// The CLI uses this client for create / list / get / update-config operations.
// It is intentionally minimal: no auth (platform-internal call), no retries
// (tenant-service is idempotent enough for the operations we expose, and
// the CLI is interactive — a human can re-run on transient failure).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Tenant mirrors the JSON shape returned by tenant-service.
// We define it here (not import from services/tenant-service) to keep
// the CLI buildable without pulling in the service module.
type Tenant struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	BIK            string    `json:"bik"`
	INN            string    `json:"inn"`
	Status         string    `json:"status"`
	DeploymentMode string    `json:"deployment_mode"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateRequest matches services/tenant-service/internal/handler.createTenantRequest.
type CreateRequest struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BIK            string `json:"bik"`
	INN            string `json:"inn"`
	DeploymentMode string `json:"deployment_mode"`
}

// UpdateConfigRequest matches services/tenant-service/internal/handler.updateConfigRequest.
type UpdateConfigRequest struct {
	RawConfig     []byte `json:"raw_config"`
	SchemaVersion string `json:"schema_version"`
}

// APIError captures a non-2xx tenant-service response.
type APIError struct {
	Status  int
	Code    string
	Message string
	Body    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("tenant-service: HTTP %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("tenant-service: HTTP %d: %s", e.Status, e.Body)
}

// Client is a thin wrapper around the tenant-service base URL.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New constructs a Client. baseURL must NOT end with a slash; trailing slashes
// are trimmed defensively.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

// WithHTTPClient overrides the underlying http.Client (used by tests).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.hc = hc
	return c
}

// Create sends POST /v1/tenants and decodes the created tenant.
// Returns *APIError on non-201 responses.
func (c *Client) Create(ctx context.Context, req CreateRequest) (*Tenant, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/tenants", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, decodeAPIError(resp)
	}
	var t Tenant
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &t, nil
}

// listResponse mirrors {"items":[...], "count":N}.
type listResponse struct {
	Items []Tenant `json:"items"`
	Count int      `json:"count"`
}

// List sends GET /v1/tenants. statusFilter is optional ("" disables it).
// Filtering is performed client-side because the current API does not accept
// a status query param (TODO server-side filter once contract supports it).
func (c *Client) List(ctx context.Context, statusFilter string, limit int) ([]Tenant, error) {
	url := c.baseURL + "/v1/tenants"
	if limit > 0 {
		url = fmt.Sprintf("%s?limit=%d", url, limit)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}
	var out listResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if statusFilter == "" {
		return out.Items, nil
	}
	filtered := make([]Tenant, 0, len(out.Items))
	for _, t := range out.Items {
		if t.Status == statusFilter {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// Get sends GET /v1/tenants/{id}.
func (c *Client) Get(ctx context.Context, id string) (*Tenant, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/v1/tenants/"+id, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}
	var t Tenant
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &t, nil
}

// UpdateConfig sends PUT /v1/tenants/{id}/config with a gzip+base64 payload
// already produced by the caller.  The handler accepts raw bytes — we just
// pass them through.
func (c *Client) UpdateConfig(ctx context.Context, id string, req UpdateConfigRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/v1/tenants/"+id+"/config", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return decodeAPIError(resp)
	}
	return nil
}

// decodeAPIError parses {"error":{"code":..,"message":..}} or falls back to raw body.
func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	apiErr := &APIError{Status: resp.StatusCode, Body: string(body)}
	var wire struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &wire) == nil {
		apiErr.Code = wire.Error.Code
		apiErr.Message = wire.Error.Message
	}
	return apiErr
}

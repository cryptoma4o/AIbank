package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	tenantID   string
}

func NewClient(baseURL, tenantID string) *Client {
	return &Client{
		baseURL:    baseURL,
		tenantID:   tenantID,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Log(ctx context.Context, eventType EventType, actorID, actorRole, resourceType, resourceID string, payload map[string]any) error {
	event := AuditEvent{
		TenantID:     c.tenantID,
		EventType:    eventType,
		ActorID:      actorID,
		ActorRole:    actorRole,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Payload:      payload,
		OccurredAt:   time.Now().UTC(),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("audit: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("audit: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("audit: unexpected status %d", resp.StatusCode)
	}
	return nil
}

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AuditEvent — payload audit-service'а.
type AuditEvent struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	ActorType   string          `json:"actor_type"`
	ActorID     string          `json:"actor_id"`
	Action      string          `json:"action"`
	SubjectType string          `json:"subject_type"`
	SubjectID   string          `json:"subject_id"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// AuditClient — клиент к audit-service.
type AuditClient struct {
	baseURL string
	tr      *transport
}

func NewAuditClient(baseURL string) *AuditClient {
	return &AuditClient{baseURL: baseURL, tr: newTransport()}
}

// AuditFilter — параметры выборки.
type AuditFilter struct {
	Actor  string
	Action string
	Limit  int
}

// List — GET /v1/audit-events?tenant_id=...
func (c *AuditClient) List(ctx context.Context, tenantID string, f AuditFilter) ([]AuditEvent, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	if f.Actor != "" {
		q.Set("actor", f.Actor)
	}
	if f.Action != "" {
		q.Set("action", f.Action)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	u := fmt.Sprintf("%s/v1/audit-events?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Tenant-Id", tenantID)
	var resp struct {
		Items []AuditEvent `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

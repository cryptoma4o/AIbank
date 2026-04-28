package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Application — admin-payload заявки.
type Application struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ApplicantID     string    `json:"applicant_id"`
	LegalEntityType string    `json:"legal_entity_type"`
	Channel         string    `json:"channel"`
	State           string    `json:"state"`
	ProductCodes    []string  `json:"product_codes"`
	WorkflowID      string    `json:"workflow_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Decision — финальное решение по заявке.
type Decision struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Decision      string    `json:"decision"`
	Reasoning     string    `json:"reasoning"`
	DecidedAt     time.Time `json:"decided_at"`
}

// OrchestratorClient — клиент к onboarding-orchestrator.
type OrchestratorClient struct {
	baseURL string
	tr      *transport
}

func NewOrchestratorClient(baseURL string) *OrchestratorClient {
	return &OrchestratorClient{baseURL: baseURL, tr: newTransport()}
}

// ListFilter — параметры фильтрации.
type ListFilter struct {
	State           string
	LegalEntityType string
	Limit           int
}

// List — GET /v1/applications?tenant_id=...
func (c *OrchestratorClient) List(ctx context.Context, tenantID string, f ListFilter) ([]Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	if f.State != "" {
		q.Set("state", f.State)
	}
	if f.LegalEntityType != "" {
		q.Set("legal_entity_type", f.LegalEntityType)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	u := fmt.Sprintf("%s/v1/applications?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var resp struct {
		Items []Application `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// ListByDateRange — заявки тенанта с фильтром по created_at ∈ [from, to].
//
// Сегодня orchestrator не принимает date-фильтр на /v1/applications,
// поэтому фильтрация выполняется client-side.  TODO upstream: добавить
// query-параметры created_from/created_to, чтобы избежать выгрузки всего
// списка для тенантов с большим объёмом заявок.
func (c *OrchestratorClient) ListByDateRange(ctx context.Context, tenantID string, from, to time.Time) ([]Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	// Передаём фильтр на сервер (на случай если upstream уже умеет — экономим payload),
	// но всё равно делаем повторную фильтрацию на клиенте — она безопасна.
	if !from.IsZero() {
		q.Set("created_from", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("created_to", to.UTC().Format(time.RFC3339))
	}
	u := fmt.Sprintf("%s/v1/applications?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var resp struct {
		Items []Application `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := resp.Items[:0]
	for _, a := range resp.Items {
		if !from.IsZero() && a.CreatedAt.Before(from) {
			continue
		}
		if !to.IsZero() && a.CreatedAt.After(to) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Get — GET /v1/applications/{id}?tenant_id=...
func (c *OrchestratorClient) Get(ctx context.Context, tenantID, id string) (*Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/applications/%s?%s", c.baseURL, id, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var p Application
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateDecision — POST /v1/applications/{id}/decision.
//
// Используется для ручных решений в manual_review состоянии.
func (c *OrchestratorClient) UpdateDecision(ctx context.Context, tenantID, applicationID, decision, reasoning string) (*Decision, error) {
	u := fmt.Sprintf("%s/v1/applications/%s/decision", c.baseURL, applicationID)
	body := map[string]any{
		"tenant_id": tenantID,
		"decision":  decision,
		"reasoning": reasoning,
	}
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	var d Decision
	if err := c.tr.doJSON(req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

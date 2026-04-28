package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// RiskAssessment — admin-payload скоринга.
type RiskAssessment struct {
	ID             string            `json:"id"`
	ApplicationID  string            `json:"application_id"`
	Score          float64           `json:"score"`
	Category       string            `json:"category"`
	Recommendation string            `json:"recommendation"`
	ComputedAt     time.Time         `json:"computed_at"`
	Factors        []json.RawMessage `json:"factors,omitempty"`
}

// RiskClient — клиент к risk-engine.
type RiskClient struct {
	baseURL string
	tr      *transport
}

func NewRiskClient(baseURL string) *RiskClient {
	return &RiskClient{baseURL: baseURL, tr: newTransport()}
}

// GetByApplication — (nil,nil) если оценки ещё нет.
func (c *RiskClient) GetByApplication(ctx context.Context, tenantID, applicationID string) (*RiskAssessment, error) {
	q := url.Values{}
	q.Set("application_id", applicationID)
	u := fmt.Sprintf("%s/v1/risk-assessments?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Tenant-Id", tenantID)
	var p RiskAssessment
	if err := c.tr.doJSON(req, &p); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if p.ID == "" {
		return nil, nil
	}
	return &p, nil
}

package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"aibank/bff-onboarding/internal/model"
)

// RiskClient — клиент к risk-engine.
type RiskClient struct {
	baseURL string
	tr      *transport
}

func NewRiskClient(baseURL string) *RiskClient {
	return &RiskClient{baseURL: baseURL, tr: newTransport()}
}

type riskPayload struct {
	ID             string                   `json:"id"`
	ApplicationID  string                   `json:"application_id"`
	Score          float64                  `json:"score"`
	Category       string                   `json:"category"`
	Recommendation string                   `json:"recommendation"`
	ComputedAt     time.Time                `json:"computed_at"`
	Factors        []map[string]interface{} `json:"factors,omitempty"`
}

// GetByApplication — GET /v1/risk-assessments?application_id=...
//
// Возвращает (nil, nil) если оценки ещё нет (заявка не дошла до
// risk_assessing).  Это нормальное состояние, а не ошибка.
func (c *RiskClient) GetByApplication(ctx context.Context, tenantID, applicationID string) (*model.RiskAssessment, error) {
	q := url.Values{}
	q.Set("application_id", applicationID)
	u := fmt.Sprintf("%s/v1/risk-assessments?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Tenant-Id", tenantID)

	var p riskPayload
	err = c.tr.doJSON(req, &p)
	if err != nil {
		// 404 = ещё нет оценки; не ошибка для GraphQL-резолвера.
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if p.ID == "" {
		return nil, nil
	}
	return &model.RiskAssessment{
		ID:             p.ID,
		ApplicationID:  p.ApplicationID,
		Score:          p.Score,
		Category:       model.RiskCategory(p.Category),
		Recommendation: p.Recommendation,
		ComputedAt:     p.ComputedAt,
		Factors:        p.Factors,
	}, nil
}

func isNotFound(err error) bool {
	return err != nil && (err == ErrNotFound || (err.Error() != "" && contains(err.Error(), "not found")))
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

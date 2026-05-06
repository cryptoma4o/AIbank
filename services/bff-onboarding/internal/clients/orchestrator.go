package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"aibank/bff-onboarding/internal/model"
)

// ApplicantHasApplicationError возвращается из Submit, когда orchestrator
// вернул 409 Conflict из-за бизнес-правила "одна заявка на applicant".
// Несёт ID существующей заявки, чтобы фронт мог редиректить пользователя.
type ApplicantHasApplicationError struct {
	ExistingApplicationID string
	ExistingState         string
}

func (e *ApplicantHasApplicationError) Error() string {
	return fmt.Sprintf("applicant already has application %s (state=%s)",
		e.ExistingApplicationID, e.ExistingState)
}

// Extensions реализует gqlerrors.ExtendedError — graphql-go автоматически
// засунет эти поля в extensions JSON-ответа, чтобы фронт мог распарсить
// existing_application_id и редиректить на уже идущую заявку.
func (e *ApplicantHasApplicationError) Extensions() map[string]interface{} {
	return map[string]interface{}{
		"code":                    "APPLICANT_HAS_APPLICATION",
		"existingApplicationId":   e.ExistingApplicationID,
		"existingState":           e.ExistingState,
	}
}

// OrchestratorClient — клиент к onboarding-orchestrator.
type OrchestratorClient struct {
	baseURL string
	tr      *transport
}

func NewOrchestratorClient(baseURL string) *OrchestratorClient {
	return &OrchestratorClient{baseURL: baseURL, tr: newTransport()}
}

// applicationPayload зеркалит onboarding-orchestrator/internal/domain.Application.
type applicationPayload struct {
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

// GetApplication — GET /v1/applications/{id}?tenant_id=...
func (c *OrchestratorClient) GetApplication(ctx context.Context, tenantID, id string) (*model.Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/applications/%s?%s", c.baseURL, id, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var p applicationPayload
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return c.toModel(&p), nil
}

// ListByApplicant — GET /v1/applications?applicant_id=...
//
// TODO: эндпоинт ещё не реализован в orchestrator (см. handler/application.go).
// Здесь — клиентская сторона; добавить серверную в отдельной задаче.
func (c *OrchestratorClient) ListByApplicant(ctx context.Context, tenantID, applicantID string) ([]model.Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	q.Set("applicant_id", applicantID)
	u := fmt.Sprintf("%s/v1/applications?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var resp struct {
		Items []applicationPayload `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := make([]model.Application, 0, len(resp.Items))
	for i := range resp.Items {
		out = append(out, *c.toModel(&resp.Items[i]))
	}
	return out, nil
}

// SubmitInput — тело POST /v1/applications.
type SubmitInput struct {
	TenantID        string         `json:"tenant_id"`
	ApplicantID     string         `json:"applicant_id"`
	LegalEntityType string         `json:"legal_entity_type"`
	Channel         string         `json:"channel"`
	ProductCodes    []string       `json:"product_codes"`
	RiskThresholds  map[string]any `json:"risk_thresholds"`
}

// Submit — POST /v1/applications, стартует Temporal-workflow.
func (c *OrchestratorClient) Submit(ctx context.Context, in SubmitInput) (*model.Application, error) {
	u := fmt.Sprintf("%s/v1/applications", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, in)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ApplicationID string `json:"application_id"`
		WorkflowID    string `json:"workflow_id"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		// Маппинг 409 (правило "одна заявка на applicant") в типизированную
		// ошибку, чтобы resolver проброcил её в GraphQL extensions.
		var se *StatusError
		if errors.As(err, &se) && se.StatusCode == http.StatusConflict {
			var conflict struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
				ExistingApplicationID string `json:"existing_application_id"`
				ExistingState         string `json:"existing_state"`
			}
			if jerr := json.Unmarshal(se.Body, &conflict); jerr == nil &&
				conflict.Error.Code == "applicant_has_application" {
				return nil, &ApplicantHasApplicationError{
					ExistingApplicationID: conflict.ExistingApplicationID,
					ExistingState:         conflict.ExistingState,
				}
			}
		}
		return nil, err
	}
	// Сразу подгружаем полный объект, чтобы вернуть фронту консистентный shape.
	return c.GetApplication(ctx, in.TenantID, resp.ApplicationID)
}

// SignalDocumentsUploaded — POST /v1/applications/{id}/signals/documents-uploaded.
func (c *OrchestratorClient) SignalDocumentsUploaded(ctx context.Context, tenantID, applicationID string, documents []map[string]any) error {
	u := fmt.Sprintf("%s/v1/applications/%s/signals/documents-uploaded", c.baseURL, applicationID)
	body := map[string]any{
		"tenant_id": tenantID,
		"documents": documents,
	}
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	return c.tr.doJSON(req, nil)
}

func (c *OrchestratorClient) toModel(p *applicationPayload) *model.Application {
	return &model.Application{
		ID:              p.ID,
		TenantID:        p.TenantID,
		State:           model.ApplicationState(p.State),
		LegalEntityType: p.LegalEntityType,
		Channel:         p.Channel,
		ProductCodes:    p.ProductCodes,
		ApplicantID:     p.ApplicantID,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

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

// PrequalifyInput — тело POST /v1/prequalify (этап 1 формы онбординга).
type PrequalifyInput struct {
	TenantID  string `json:"tenant_id"`
	INN       string `json:"inn"`
	OGRN      string `json:"ogrn"`
	ShortName string `json:"short_name"`
}

// PrequalifyResult зеркалит ответ orchestrator'а — см.
// services/onboarding-orchestrator/internal/handler/prequalify.go.PrequalifyResponse.
type PrequalifyResult struct {
	INN                   string    `json:"inn"`
	OGRN                  string    `json:"ogrn"`
	ShortNameHint         string    `json:"short_name_hint"`
	EGRULStatus           string    `json:"egrul_status"`
	EGRULRegistrationDate string    `json:"egrul_registration_date"`
	EGRULAddress          string    `json:"egrul_address"`
	EGRULCEOName          string    `json:"egrul_ceo_name"`
	EGRULFullName         string    `json:"egrul_full_name"`
	NameMatchesEGRUL      bool      `json:"name_matches_egrul"`
	RosfinmonPresent      bool      `json:"rosfinmon_present"`
	FSSPProceedingsCount  int       `json:"fssp_proceedings_count"`
	FSSPTotalDebtKopecks  int64     `json:"fssp_total_debt_kopecks"`
	Decision              string    `json:"decision"`
	DecisionReason        string    `json:"decision_reason"`
	UnavailableSources    []string  `json:"unavailable_sources"`
	CheckedAt             time.Time `json:"checked_at"`
}

// Prequalify — POST /v1/prequalify (этап 1 формы онбординга, см.
// docs/onboarding-form-spec.md §1).
func (c *OrchestratorClient) Prequalify(ctx context.Context, in PrequalifyInput) (*PrequalifyResult, error) {
	u := fmt.Sprintf("%s/v1/prequalify", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, in)
	if err != nil {
		return nil, err
	}
	var out PrequalifyResult
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

// ── Этапы 2-10 формы онбординга ──────────────────────────────────────
//
// Все методы ниже — тонкая обёртка над REST-эндпоинтами orchestrator'а:
//   - тело запроса передаётся как map[string]any (форму определяет
//     соответствующий handler в services/onboarding-orchestrator);
//   - tenant_id всегда из ac.TenantID, никогда из аргументов мутации;
//   - результат — map[string]any, чтобы резолверам не приходилось
//     дублировать большие structs из onboarding-orchestrator/internal/domain.

// SubmitLegalEntityProfile — POST /v1/legal-entity-profiles (этап 2).
func (c *OrchestratorClient) SubmitLegalEntityProfile(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/legal-entity-profiles", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLegalEntityProfile — GET /v1/legal-entity-profiles/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetLegalEntityProfile(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/legal-entity-profiles/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubmitApplicationActivity — POST /v1/application-activities (этап 3).
func (c *OrchestratorClient) SubmitApplicationActivity(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/application-activities", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetApplicationActivity — GET /v1/application-activities/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetApplicationActivity(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/application-activities/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertRepresentative — POST /v1/representatives (этап 4).
func (c *OrchestratorClient) UpsertRepresentative(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/representatives", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListRepresentatives — GET /v1/representatives/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) ListRepresentatives(ctx context.Context, tenantID, applicationID string) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/representatives/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	if resp.Items == nil {
		return []map[string]any{}, nil
	}
	return resp.Items, nil
}

// GetRepresentative — GET /v1/representatives/{id}?tenant_id=...
func (c *OrchestratorClient) GetRepresentative(ctx context.Context, tenantID, id string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/representatives/%s?%s", c.baseURL, id, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertUBOGraph — POST /v1/ubo-graphs (этап 5).
func (c *OrchestratorClient) UpsertUBOGraph(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/ubo-graphs", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetUBOGraph — GET /v1/ubo-graphs/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetUBOGraph(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/ubo-graphs/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertScreening — POST /v1/screenings (этап 7).
func (c *OrchestratorClient) UpsertScreening(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/screenings", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetScreening — GET /v1/screenings/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetScreening(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/screenings/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertMonitoringProfile — POST /v1/monitoring-profiles (этапы 8/10).
func (c *OrchestratorClient) UpsertMonitoringProfile(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/monitoring-profiles", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMonitoringProfile — GET /v1/monitoring-profiles/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetMonitoringProfile(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/monitoring-profiles/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertAccount — POST /v1/accounts (этап 9).
func (c *OrchestratorClient) UpsertAccount(ctx context.Context, tenantID string, body map[string]any) (map[string]any, error) {
	body["tenant_id"] = tenantID
	u := fmt.Sprintf("%s/v1/accounts", c.baseURL)
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAccount — GET /v1/accounts/by-application/{applicationID}?tenant_id=...
func (c *OrchestratorClient) GetAccount(ctx context.Context, tenantID, applicationID string) (map[string]any, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/accounts/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	out := map[string]any{}
	if err := c.tr.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
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

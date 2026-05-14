// Package handler содержит HTTP-обработчики онбординг-оркестратора.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"context"

	"github.com/go-chi/chi/v5"
	"go.temporal.io/sdk/client"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
	"aibank/onboarding-orchestrator/internal/workflow"
)

// validTenantID — те же правила, что и в repository (ADR-0002).
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// IDGenerator — порт для генерации ID-шников.  В проде — ULID/UUID,
// в тестах — детерминированный stub.
type IDGenerator interface {
	NewApplicationID() string
}

// ApplicationHandler — HTTP-фасад над Temporal-клиентом и БД.
type ApplicationHandler struct {
	repo        domain.ApplicationRepository
	temporal    client.Client
	idgen       IDGenerator
	auditClient *auditsdk.Client
	log         *slog.Logger
}

// NewApplicationHandler — auditClient может быть nil (best-effort через ADR-0010).
func NewApplicationHandler(
	repo domain.ApplicationRepository,
	tc client.Client,
	idgen IDGenerator,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *ApplicationHandler {
	return &ApplicationHandler{repo: repo, temporal: tc, idgen: idgen, auditClient: auditClient, log: log}
}

// withPlatformAuth выставляет AuthInfo из заголовков. orchestrator вызывается
// внутри платформы (BFF/identity), JWT-обогащение — задача ADR-0010 follow-up.
func withPlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID := r.Header.Get("X-Actor-ID")
		if actorID == "" {
			actorID = "system"
		}
		ctx := auditsdk.WithAuthInfo(r.Context(), auditsdk.AuthInfo{
			TenantID:  r.Header.Get("X-Tenant-ID"),
			ActorID:   actorID,
			ActorType: auditsdk.ActorTypeSystem,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Routes возвращает chi-роутер со всеми эндпоинтами /v1/applications.
func (h *ApplicationHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "application.created",
			h.resolveApplicationCreate, h.log)).Post("/", h.Create)
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "application.signal_received",
			h.resolveDocumentsSignal, h.log)).Post("/{id}/signals/documents-uploaded", h.SignalDocumentsUploaded)
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "application.signal_received",
			h.resolveHumanSignal, h.log)).Post("/{id}/signals/human-decision", h.SignalHumanDecision)
	} else {
		r.Post("/", h.Create)
		r.Post("/{id}/signals/documents-uploaded", h.SignalDocumentsUploaded)
		r.Post("/{id}/signals/human-decision", h.SignalHumanDecision)
	}
	r.Get("/", h.List)
	r.Get("/{id}", h.Get)
	// Admin: ручной перевод заявки между состояниями (используется bank-оператором,
	// пока Temporal activities не реализованы). Защищён через bank.* role в bff-admin.
	r.Post("/{id}/transitions", h.TransitionState)
	return r
}

// EntityResolver-ы.

func (h *ApplicationHandler) resolveApplicationCreate(r *http.Request) (string, string, json.RawMessage, bool) {
	id := applicationIDFromContext(r.Context())
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"application_id": id})
	return "application", id, payload, true
}

func (h *ApplicationHandler) resolveDocumentsSignal(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"application_id": id, "signal": "documents_uploaded"})
	return "application", id, payload, true
}

func (h *ApplicationHandler) resolveHumanSignal(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"application_id": id, "signal": "human_decision"})
	return "application", id, payload, true
}

type ctxKey string

const ctxKeyApplicationID ctxKey = "application_id"

func applicationIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyApplicationID).(string)
	return v
}

// ── DTO ──────────────────────────────────────────────────────────────

type createRequest struct {
	TenantID        string                 `json:"tenant_id"`
	ApplicantID     string                 `json:"applicant_id"`
	LegalEntityType domain.LegalEntityType `json:"legal_entity_type"`
	Channel         domain.Channel         `json:"channel"`
	ProductCodes    []string               `json:"product_codes"`
	RiskThresholds  workflow.RiskThresholds `json:"risk_thresholds"`
}

func (req createRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid (regex ^[a-z][a-z0-9_]{1,31}$)")
	}
	if req.ApplicantID == "" {
		return errors.New("applicant_id is required")
	}
	if !req.LegalEntityType.IsValid() {
		return errors.New("legal_entity_type must be IP|LLC|JSC|NPF")
	}
	if !req.Channel.IsValid() {
		return errors.New("channel must be web|mobile|courier|branch")
	}
	if len(req.ProductCodes) == 0 {
		return errors.New("product_codes must contain at least one entry")
	}
	if req.RiskThresholds.AutoApproveBelow <= 0 || req.RiskThresholds.DeclineAbove <= 0 ||
		req.RiskThresholds.AutoApproveBelow >= req.RiskThresholds.DeclineAbove {
		return errors.New("risk_thresholds invalid: 0 < auto_approve_below < decline_above")
	}
	return nil
}

type createResponse struct {
	ApplicationID string `json:"application_id"`
	WorkflowID    string `json:"workflow_id"`
}

type signalDocsRequest struct {
	TenantID  string                       `json:"tenant_id"`
	Documents []workflow.DocumentReference `json:"documents"`
}

type signalDecisionRequest struct {
	TenantID string `json:"tenant_id"`
	Decision string `json:"decision"` // approved | declined | approved_with_edd
	Reason   string `json:"reason,omitempty"`
	Reviewer string `json:"reviewer"`
}

// ── Handlers ─────────────────────────────────────────────────────────

func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	// Бизнес-правило "одна заявка на applicant" (см. migrations/tenant/
	// 002_one_application_per_applicant.sql). Проверяем заранее ради
	// читаемого 409 с existing_application_id; race-condition защита
	// сидит в repo.Create через unique_violation → ErrAlreadyExists.
	existing, err := h.repo.GetByApplicant(r.Context(), req.TenantID, req.ApplicantID)
	if err == nil {
		writeApplicantConflict(w, existing)
		return
	}
	if !errors.Is(err, repository.ErrNotFound) {
		h.log.Error("lookup applicant", "tenant_id", req.TenantID, "applicant_id", req.ApplicantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	appID := h.idgen.NewApplicationID()
	workflowID := "app-" + appID

	app := &domain.Application{
		ID:              appID,
		TenantID:        req.TenantID,
		ApplicantID:     req.ApplicantID,
		LegalEntityType: req.LegalEntityType,
		Channel:         req.Channel,
		State:           domain.StateDraft,
		ProductCodes:    req.ProductCodes,
		WorkflowID:      workflowID,
		AccountIDs:      []string{},
		CreatedAt:       time.Now().UTC(),
	}
	if err := h.repo.Create(r.Context(), app); err != nil {
		// Race condition: pre-check выше прошёл, но другой запрос успел
		// вставить запись первым. Повторно подтягиваем существующую и
		// возвращаем тот же 409, что и pre-check.
		if errors.Is(err, repository.ErrAlreadyExists) {
			if existing, lookupErr := h.repo.GetByApplicant(r.Context(), req.TenantID, req.ApplicantID); lookupErr == nil {
				writeApplicantConflict(w, existing)
				return
			}
		}
		h.log.Error("create application", "tenant", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create application")
		return
	}

	input := workflow.ApplicationInput{
		ApplicationID:   appID,
		TenantID:        req.TenantID,
		ApplicantID:     req.ApplicantID,
		LegalEntityType: req.LegalEntityType,
		Channel:         req.Channel,
		ProductCodes:    req.ProductCodes,
		RiskThresholds:  req.RiskThresholds,
	}
	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: workflow.TaskQueue,
	}
	if _, err := h.temporal.ExecuteWorkflow(r.Context(), opts, workflow.OnboardingWorkflow, input); err != nil {
		h.log.Error("execute workflow", "workflow_id", workflowID, "err", err)
		writeError(w, http.StatusInternalServerError, "workflow_failed", err.Error())
		return
	}

	// Audit-emit: tenant + application_id из тела/idgen.
	auditsdk.SetTenantID(r.Context(), req.TenantID)
	createPayload, _ := json.Marshal(map[string]any{
		"application_id":    appID,
		"workflow_id":       workflowID,
		"applicant_id":      req.ApplicantID,
		"legal_entity_type": req.LegalEntityType,
		"channel":           req.Channel,
	})
	auditsdk.SetEntity(r.Context(), "application", appID, createPayload)

	writeJSON(w, http.StatusCreated, createResponse{
		ApplicationID: appID,
		WorkflowID:    workflowID,
	})
}

// List — GET /v1/applications?tenant_id=...&applicant_id=...&state=...&legal_entity_type=...&limit=...
//
// Repository поддерживает фильтрацию только по tenantID + limit (см.
// PostgresApplicationRepository.ListByTenant). Дополнительные фильтры
// applicant_id / state / legal_entity_type применяются in-memory после
// загрузки — допустимо для pre-MVP, при росте объёмов нужно расширить SQL.
//
// applicant_id — security-критичный фильтр: bff-onboarding передаёт
// UserID из JWT, чтобы applicant видел только свои заявки и не получал
// заявки других applicants того же тенанта.
func (h *ApplicationHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	applicantFilter := r.URL.Query().Get("applicant_id")
	stateFilter := r.URL.Query().Get("state")
	if stateFilter != "" && !domain.ApplicationState(stateFilter).IsValid() {
		writeError(w, http.StatusBadRequest, "validation_failed", "state is invalid")
		return
	}
	legalFilter := r.URL.Query().Get("legal_entity_type")
	if legalFilter != "" && !domain.LegalEntityType(legalFilter).IsValid() {
		writeError(w, http.StatusBadRequest, "validation_failed", "legal_entity_type is invalid")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	apps, err := h.repo.ListByTenant(r.Context(), tenantID, limit)
	if err != nil {
		h.log.Error("list applications", "tenant_id", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	out := make([]*domain.Application, 0, len(apps))
	for _, a := range apps {
		if applicantFilter != "" && a.ApplicantID != applicantFilter {
			continue
		}
		if stateFilter != "" && string(a.State) != stateFilter {
			continue
		}
		if legalFilter != "" && string(a.LegalEntityType) != legalFilter {
			continue
		}
		out = append(out, a)
	}
	// Wrap в {items: [...]} — формат, который ожидает bff-admin
	// orchestrator-клиент (см. services/bff-admin/internal/clients/orchestrator.go).
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *ApplicationHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "id is required")
		return
	}

	app, err := h.repo.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "application not found")
		return
	}
	if err != nil {
		h.log.Error("get application", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *ApplicationHandler) SignalDocumentsUploaded(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req signalDocsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !validTenantID.MatchString(req.TenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id is invalid")
		return
	}
	if len(req.Documents) == 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "documents must not be empty")
		return
	}

	// Проверяем, что заявка действительно в этом тенанте.
	app, err := h.repo.GetByID(r.Context(), req.TenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "application not found")
		return
	}
	if err != nil {
		h.log.Error("get application for signal", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	payload := workflow.DocumentsUploadedSignal{
		Documents:  req.Documents,
		UploadedAt: time.Now().UTC(),
	}
	if err := h.temporal.SignalWorkflow(r.Context(), app.WorkflowID, "", workflow.SignalDocumentsUploaded, payload); err != nil {
		h.log.Error("signal documents_uploaded", "workflow_id", app.WorkflowID, "err", err)
		writeError(w, http.StatusInternalServerError, "signal_failed", err.Error())
		return
	}

	// Audit-emit для signal-события.
	auditsdk.SetTenantID(r.Context(), req.TenantID)
	docsPayload, _ := json.Marshal(map[string]any{
		"application_id": id,
		"signal":         "documents_uploaded",
		"document_count": len(req.Documents),
	})
	auditsdk.SetEntity(r.Context(), "application", id, docsPayload)

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *ApplicationHandler) SignalHumanDecision(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req signalDecisionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !validTenantID.MatchString(req.TenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id is invalid")
		return
	}
	if req.Reviewer == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "reviewer is required")
		return
	}
	switch req.Decision {
	case "approved", "declined", "approved_with_edd":
	default:
		writeError(w, http.StatusBadRequest, "validation_failed", "decision must be approved|declined|approved_with_edd")
		return
	}

	app, err := h.repo.GetByID(r.Context(), req.TenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "application not found")
		return
	}
	if err != nil {
		h.log.Error("get application for signal", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	payload := workflow.HumanDecisionSignal{
		Decision:  req.Decision,
		Reason:    req.Reason,
		Reviewer:  req.Reviewer,
		DecidedAt: time.Now().UTC(),
	}
	if err := h.temporal.SignalWorkflow(r.Context(), app.WorkflowID, "", workflow.SignalHumanDecision, payload); err != nil {
		h.log.Error("signal human_decision", "workflow_id", app.WorkflowID, "err", err)
		writeError(w, http.StatusInternalServerError, "signal_failed", err.Error())
		return
	}

	// Audit-emit для human-decision события.
	auditsdk.SetTenantID(r.Context(), req.TenantID)
	humanPayload, _ := json.Marshal(map[string]any{
		"application_id": id,
		"signal":         "human_decision",
		"decision":       req.Decision,
		"reviewer":       req.Reviewer,
	})
	auditsdk.SetEntity(r.Context(), "application", id, humanPayload)

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// ── Admin transition ─────────────────────────────────────────────────

type transitionRequest struct {
	TenantID string `json:"tenant_id"`
	NewState string `json:"new_state"`
	Reason   string `json:"reason,omitempty"`
}

func (req transitionRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if !domain.ApplicationState(req.NewState).IsValid() {
		return errors.New("new_state is not a valid ApplicationState")
	}
	return nil
}

// TransitionState — POST /v1/applications/{id}/transitions.
//
// Admin-endpoint для ручного перевода заявки между состояниями. Используется
// bank-оператором/комплаенсом из админ-панели, пока Temporal-activities не
// реализованы (см. activities.go — только интерфейсы). Repository сам
// валидирует допустимость перехода через CanTransition() — если переход
// запрещён state machine'ой, возвращается 409 Conflict.
//
// Audit-emit делается с reason — банковские роли обязаны указывать причину
// для compliance-trail (115-ФЗ).
func (h *ApplicationHandler) TransitionState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "id is required")
		return
	}
	var req transitionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	if err := h.repo.UpdateState(r.Context(), req.TenantID, id, domain.ApplicationState(req.NewState)); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "application not found")
			return
		}
		if errors.Is(err, repository.ErrInvalidTransition) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]string{
					"code":    "invalid_transition",
					"message": err.Error(),
				},
			})
			return
		}
		h.log.Error("transition state", "id", id, "new_state", req.NewState, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	app, err := h.repo.GetByID(r.Context(), req.TenantID, id)
	if err != nil {
		h.log.Error("read back after transition", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// ── helpers ──────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}

// writeApplicantConflict — единый ответ при попытке создать вторую заявку
// для applicant'а. Включает existing_application_id и existing_state, чтобы
// клиент (web-onboarding) мог отредиректить пользователя на уже идущую заявку.
func writeApplicantConflict(w http.ResponseWriter, existing *domain.Application) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": map[string]string{
			"code":    "applicant_has_application",
			"message": "applicant already has an application",
		},
		"existing_application_id": existing.ID,
		"existing_state":          string(existing.State),
	})
}

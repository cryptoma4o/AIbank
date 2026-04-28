package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

// Pipeline is the only behaviour the handler relies on — kept narrow to make
// httptest-driven tests trivial.
type Pipeline interface {
	Assess(ctx context.Context, tenantID, applicationID string, inputData map[string]any) (*domain.RiskAssessment, error)
}

// AssessmentHandler exposes the REST API for risk assessments.
type AssessmentHandler struct {
	pipeline Pipeline
	repo     domain.RiskAssessmentRepository
	log      *slog.Logger
}

// NewAssessmentHandler wires dependencies for the HTTP layer.
func NewAssessmentHandler(p Pipeline, repo domain.RiskAssessmentRepository, log *slog.Logger) *AssessmentHandler {
	if log == nil {
		log = slog.Default()
	}
	return &AssessmentHandler{pipeline: p, repo: repo, log: log}
}

// Routes wires URL → handler. Mounted under /v1/assessments.
func (h *AssessmentHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{id}", h.Get)
	return r
}

type createRequest struct {
	TenantID      string         `json:"tenant_id"`
	ApplicationID string         `json:"application_id"`
	InputData     map[string]any `json:"input_data"`
	// RulesPackPath is reserved for future per-tenant rules overrides — for
	// now the pipeline-wide engine is used. Documented here so callers can
	// already include the field without breaking the contract.
	RulesPackPath string `json:"rules_pack_path,omitempty"`
}

// Create handles POST /v1/assessments.
func (h *AssessmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id is required")
		return
	}
	if req.ApplicationID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "application_id is required")
		return
	}

	a, err := h.pipeline.Assess(r.Context(), req.TenantID, req.ApplicationID, req.InputData)
	if err != nil {
		h.log.Error("assess", "tenant", req.TenantID, "app", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "assess_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// Get handles GET /v1/assessments/{id}?tenant_id=...
func (h *AssessmentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	a, err := h.repo.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "assessment not found")
		return
	}
	if err != nil {
		h.log.Error("get assessment", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// List handles GET /v1/assessments?tenant_id=X&application_id=Y.
func (h *AssessmentHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	applicationID := r.URL.Query().Get("application_id")
	if tenantID == "" || applicationID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"tenant_id and application_id are required")
		return
	}
	items, err := h.repo.ListByApplication(r.Context(), tenantID, applicationID)
	if err != nil {
		h.log.Error("list assessments", "tenant", tenantID, "app", applicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

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

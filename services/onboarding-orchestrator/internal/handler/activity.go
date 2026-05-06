package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
)

// ActivityHandler — этап 3 формы онбординга. POST/GET /v1/application-activities.
type ActivityHandler struct {
	repo domain.ApplicationActivityRepository
	log  *slog.Logger
}

func NewActivityHandler(repo domain.ApplicationActivityRepository, log *slog.Logger) *ActivityHandler {
	return &ActivityHandler{repo: repo, log: log}
}

func (h *ActivityHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

type activityRequest struct {
	TenantID            string                    `json:"tenant_id"`
	ApplicationID       string                    `json:"application_id"`
	BusinessDescription string                    `json:"business_description"`
	BusinessCategory    string                    `json:"business_category"`
	TopSuppliers        []domain.Counterparty     `json:"top_suppliers,omitempty"`
	TopBuyers           []domain.Counterparty     `json:"top_buyers,omitempty"`
	OperationalModel    *domain.OperationalModel  `json:"operational_model,omitempty"`
	FundsSource         domain.FundsSource        `json:"funds_source"`
}

var validBusinessCategory = map[string]bool{
	"low_risk": true, "medium_risk": true, "high_risk": true,
}
var validFundsCategory = map[string]bool{
	"revenue": true, "founder_contribution": true, "loan": true, "investments": true, "other": true,
}
var validRelationship = map[string]bool{
	"regular": true, "one_off": true,
}

func (req activityRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid (regex ^[a-z][a-z0-9_]{1,31}$)")
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return errors.New("application_id is required")
	}
	if strings.TrimSpace(req.BusinessDescription) == "" {
		return errors.New("business_description is required")
	}
	if !validBusinessCategory[req.BusinessCategory] {
		return errors.New("business_category must be low_risk|medium_risk|high_risk")
	}
	if !validFundsCategory[req.FundsSource.Category] {
		return errors.New("funds_source.category must be revenue|founder_contribution|loan|investments|other")
	}
	if req.FundsSource.Category == "other" && strings.TrimSpace(req.FundsSource.Description) == "" {
		return errors.New("funds_source.description is required when category=other")
	}
	if len(req.TopSuppliers) > 5 {
		return errors.New("top_suppliers maximum is 5")
	}
	if len(req.TopBuyers) > 5 {
		return errors.New("top_buyers maximum is 5")
	}
	for i, c := range append(append([]domain.Counterparty{}, req.TopSuppliers...), req.TopBuyers...) {
		if strings.TrimSpace(c.Name) == "" {
			return errors.New("counterparty[" + intToStr(i) + "].name is required")
		}
		if c.SharePercent < 0 || c.SharePercent > 100 {
			return errors.New("counterparty[" + intToStr(i) + "].share_percent must be 0..100")
		}
		if !validRelationship[c.RelationshipType] {
			return errors.New("counterparty[" + intToStr(i) + "].relationship_type must be regular|one_off")
		}
	}
	if req.OperationalModel != nil && req.OperationalModel.CashSharePercent != nil {
		v := *req.OperationalModel.CashSharePercent
		if v < 0 || v > 100 {
			return errors.New("operational_model.cash_share_percent must be 0..100")
		}
	}
	return nil
}

// Upsert — POST /v1/application-activities.
func (h *ActivityHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req activityRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := "aac_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	a := &domain.ApplicationActivity{
		ID:                  id,
		TenantID:            req.TenantID,
		ApplicationID:       req.ApplicationID,
		BusinessDescription: req.BusinessDescription,
		BusinessCategory:    req.BusinessCategory,
		TopSuppliers:        req.TopSuppliers,
		TopBuyers:           req.TopBuyers,
		OperationalModel:    req.OperationalModel,
		FundsSource:         req.FundsSource,
		CreatedAt:           time.Now().UTC(),
	}
	if err := h.repo.Upsert(r.Context(), a); err != nil {
		h.log.Error("upsert activity", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save activity")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back activity", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

// GetByApplication — GET /v1/application-activities/by-application/{id}?tenant_id=...
func (h *ActivityHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "applicationID is required")
		return
	}
	a, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "activity not found")
		return
	}
	if err != nil {
		h.log.Error("get activity", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

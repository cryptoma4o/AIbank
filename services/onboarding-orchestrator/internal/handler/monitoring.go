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

var validKYCRefreshTrigger = map[string]bool{
	"scheduled": true, "ceo_change": true, "ubo_change": true,
	"ownership_change": true, "address_change": true, "license_expiry": true,
	"transaction_threshold_exceeded": true, "regulator_alert": true,
}
var validReviewFrequency = map[int]bool{3: true, 6: true, 12: true}

// MonitoringHandler — этапы 8 и 10 формы онбординга.
type MonitoringHandler struct {
	repo domain.MonitoringProfileRepository
	log  *slog.Logger
}

func NewMonitoringHandler(repo domain.MonitoringProfileRepository, log *slog.Logger) *MonitoringHandler {
	return &MonitoringHandler{repo: repo, log: log}
}

func (h *MonitoringHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

type monitoringRequest struct {
	TenantID              string                    `json:"tenant_id"`
	ApplicationID         string                    `json:"application_id"`
	AccountID             string                    `json:"account_id,omitempty"`
	ReviewFrequencyMonths int                       `json:"review_frequency_months"`
	NextReviewDate        string                    `json:"next_review_date"`
	MonitoringRules       []domain.MonitoringRule   `json:"monitoring_rules,omitempty"`
	KYCRefreshTriggers    []string                  `json:"kyc_refresh_triggers,omitempty"`
	TransactionLimits     *domain.TransactionLimits `json:"transaction_limits,omitempty"`
	NotificationChannels  []string                  `json:"notification_channels,omitempty"`
}

func (req monitoringRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return errors.New("application_id is required")
	}
	if !validReviewFrequency[req.ReviewFrequencyMonths] {
		return errors.New("review_frequency_months must be 3, 6 or 12")
	}
	if !validBirthDate.MatchString(req.NextReviewDate) {
		return errors.New("next_review_date must be YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", req.NextReviewDate); err != nil {
		return errors.New("next_review_date must be a valid calendar date")
	}
	for i, t := range req.KYCRefreshTriggers {
		if !validKYCRefreshTrigger[t] {
			return errors.New("kyc_refresh_triggers[" + intToStr(i) + "] invalid")
		}
	}
	for i, rule := range req.MonitoringRules {
		if strings.TrimSpace(rule.Code) == "" || strings.TrimSpace(rule.Description) == "" {
			return errors.New("monitoring_rules[" + intToStr(i) + "]: code and description required")
		}
	}
	return nil
}

func (h *MonitoringHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req monitoringRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := "mon_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	p := &domain.MonitoringProfile{
		ID:                    id,
		TenantID:              req.TenantID,
		ApplicationID:         req.ApplicationID,
		AccountID:             req.AccountID,
		ReviewFrequencyMonths: req.ReviewFrequencyMonths,
		NextReviewDate:        req.NextReviewDate,
		MonitoringRules:       req.MonitoringRules,
		KYCRefreshTriggers:    req.KYCRefreshTriggers,
		TransactionLimits:     req.TransactionLimits,
		NotificationChannels:  req.NotificationChannels,
		CreatedAt:             time.Now().UTC(),
	}
	if err := h.repo.Upsert(r.Context(), p); err != nil {
		h.log.Error("upsert monitoring", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save monitoring profile")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back monitoring", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *MonitoringHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	p, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "monitoring profile not found")
		return
	}
	if err != nil {
		h.log.Error("get monitoring", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

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

var validMatchLevel = map[string]bool{
	"match": true, "partial_match": true, "no_match": true,
}
var validAdverseCategory = map[string]bool{
	"fraud": true, "money_laundering": true, "terrorism_financing": true,
	"sanctions": true, "corruption": true, "other": true,
}

// ScreeningHandler — этап 7 формы онбординга.
type ScreeningHandler struct {
	repo domain.ScreeningResultSetRepository
	log  *slog.Logger
}

func NewScreeningHandler(repo domain.ScreeningResultSetRepository, log *slog.Logger) *ScreeningHandler {
	return &ScreeningHandler{repo: repo, log: log}
}

func (h *ScreeningHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

type screeningRequest struct {
	TenantID              string                  `json:"tenant_id"`
	ApplicationID         string                  `json:"application_id"`
	SanctionsResults      []domain.ScreeningResult `json:"sanctions_results,omitempty"`
	PEPResults            []domain.ScreeningResult `json:"pep_results,omitempty"`
	AdverseMediaHits      []domain.AdverseMediaHit `json:"adverse_media_hits,omitempty"`
	OKVEDConsistencyScore *float64                `json:"okved_consistency_score,omitempty"`
	TurnoverRealismScore  *float64                `json:"turnover_realism_score,omitempty"`
	AntiFraudSignals      *domain.AntiFraudSignals `json:"anti_fraud_signals,omitempty"`
}

func (req screeningRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return errors.New("application_id is required")
	}
	for i, sr := range append(append([]domain.ScreeningResult{}, req.SanctionsResults...), req.PEPResults...) {
		if strings.TrimSpace(sr.ListName) == "" {
			return errors.New("screening_result[" + intToStr(i) + "].list_name is required")
		}
		if !validMatchLevel[sr.MatchLevel] {
			return errors.New("screening_result[" + intToStr(i) + "].match_level must be match|partial_match|no_match")
		}
		if sr.Score < 0 || sr.Score > 1 {
			return errors.New("screening_result[" + intToStr(i) + "].score must be 0..1")
		}
	}
	for i, m := range req.AdverseMediaHits {
		if !validAdverseCategory[m.Category] {
			return errors.New("adverse_media[" + intToStr(i) + "].category invalid")
		}
		if strings.TrimSpace(m.Title) == "" || strings.TrimSpace(m.URL) == "" || strings.TrimSpace(m.PublishedAt) == "" {
			return errors.New("adverse_media[" + intToStr(i) + "]: title, url, published_at required")
		}
	}
	if req.OKVEDConsistencyScore != nil && (*req.OKVEDConsistencyScore < 0 || *req.OKVEDConsistencyScore > 100) {
		return errors.New("okved_consistency_score must be 0..100")
	}
	if req.TurnoverRealismScore != nil && (*req.TurnoverRealismScore < 0 || *req.TurnoverRealismScore > 100) {
		return errors.New("turnover_realism_score must be 0..100")
	}
	return nil
}

func (h *ScreeningHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req screeningRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := "scr_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	now := time.Now().UTC()
	s := &domain.ScreeningResultSet{
		ID:                    id,
		TenantID:              req.TenantID,
		ApplicationID:         req.ApplicationID,
		SanctionsResults:      req.SanctionsResults,
		PEPResults:            req.PEPResults,
		AdverseMediaHits:      req.AdverseMediaHits,
		OKVEDConsistencyScore: req.OKVEDConsistencyScore,
		TurnoverRealismScore:  req.TurnoverRealismScore,
		AntiFraudSignals:      req.AntiFraudSignals,
		PerformedAt:           now,
		CreatedAt:             now,
	}
	if err := h.repo.Upsert(r.Context(), s); err != nil {
		h.log.Error("upsert screening", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save screening")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back screening", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *ScreeningHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	s, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "screening result set not found")
		return
	}
	if err != nil {
		h.log.Error("get screening", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

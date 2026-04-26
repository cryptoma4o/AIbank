package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"aibank/billing-service/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type billingRepo interface {
	RecordEvent(ctx context.Context, e *domain.BillableEvent) error
	GetUsageReport(ctx context.Context, tenantID string, from, to time.Time) (*domain.UsageReport, error)
}

type BillingHandler struct {
	repo billingRepo
}

func NewBillingHandler(repo billingRepo) *BillingHandler {
	return &BillingHandler{repo: repo}
}

func (h *BillingHandler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", h.healthz)
	r.Route("/v1/billing", func(r chi.Router) {
		r.Post("/events", h.recordEvent)
		r.Get("/report/{tenant_id}", h.getReport)
	})

	return r
}

func (h *BillingHandler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *BillingHandler) recordEvent(w http.ResponseWriter, r *http.Request) {
	var e domain.BillableEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if e.TenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
		return
	}
	if e.Kind == "" {
		http.Error(w, "kind is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.RecordEvent(r.Context(), &e); err != nil {
		http.Error(w, "failed to record event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

func (h *BillingHandler) getReport(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenant_id")
	if tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			http.Error(w, "invalid 'from' date, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		from = t
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			http.Error(w, "invalid 'to' date, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		to = t
	}

	report, err := h.repo.GetUsageReport(r.Context(), tenantID, from, to)
	if err != nil {
		http.Error(w, "failed to get report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

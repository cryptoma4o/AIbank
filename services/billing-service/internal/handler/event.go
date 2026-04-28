package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/billing-service/internal/domain"
	"aibank/billing-service/internal/repository"
)

const (
	maxBodyBytes  = 64 * 1024
	defaultLimit  = 100
	maxLimit      = 1000
	dateFormatISO = "2006-01-02"
)

// TierResolver резолвит tier по tenant_id (обычно — clients.TenantClient).
type TierResolver interface {
	ResolveTier(ctx context.Context, tenantID string) domain.Tier
}

type EventHandler struct {
	repo        domain.BillingEventRepository
	tiers       TierResolver
	auditClient *auditsdk.Client
	log         *slog.Logger
}

// NewEventHandler — auditClient может быть nil (best-effort через ADR-0010).
func NewEventHandler(
	repo domain.BillingEventRepository,
	tiers TierResolver,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *EventHandler {
	return &EventHandler{repo: repo, tiers: tiers, auditClient: auditClient, log: log}
}

// withPlatformAuth выставляет AuthInfo из заголовков. billing-service пока
// без JWT (ADR-0010 follow-up).
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

func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "billing_event.recorded",
			h.resolveBillingEvent, h.log)).Post("/events", h.recordEvent)
	} else {
		r.Post("/events", h.recordEvent)
	}
	r.Get("/events", h.listEvents)
	r.Get("/events/{id}", h.getEvent)
	r.Get("/aggregate", h.aggregate)
	return r
}

// resolveBillingEvent — EntityResolver для POST /v1/events.
func (h *EventHandler) resolveBillingEvent(r *http.Request) (string, string, json.RawMessage, bool) {
	id := billingEventIDFromContext(r.Context())
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"billing_event_id": id})
	return "billing_event", id, payload, true
}

type ctxKey string

const ctxKeyBillingEventID ctxKey = "billing_event_id"

func billingEventIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyBillingEventID).(string)
	return v
}

type recordEventRequest struct {
	TenantID      string          `json:"tenant_id"`
	EventType     string          `json:"event_type"`
	SourceService string          `json:"source_service"`
	SourceEventID string          `json:"source_event_id"`
	Quantity      int             `json:"quantity,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	AuditEventID  string          `json:"audit_event_id,omitempty"`
}

func (req recordEventRequest) validate() error {
	if req.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if req.EventType == "" {
		return errors.New("event_type is required")
	}
	if req.SourceService == "" {
		return errors.New("source_service is required")
	}
	if req.SourceEventID == "" {
		return errors.New("source_event_id is required")
	}
	if req.Quantity < 0 {
		return errors.New("quantity must be >= 0")
	}
	return nil
}

func (h *EventHandler) recordEvent(w http.ResponseWriter, r *http.Request) {
	var req recordEventRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	quantity := req.Quantity
	if quantity == 0 {
		quantity = 1
	}

	tier := h.tiers.ResolveTier(r.Context(), req.TenantID)
	pricelist := domain.LoadPricelist(tier)
	unitPrice := pricelist.Price(req.EventType, h.log)

	evt := &domain.BillingEvent{
		ID:               domain.DeriveID(req.TenantID, req.EventType, req.SourceEventID),
		TenantID:         req.TenantID,
		EventType:        req.EventType,
		SourceService:    req.SourceService,
		SourceEventID:    req.SourceEventID,
		Quantity:         quantity,
		UnitPriceKopecks: unitPrice,
		TotalKopecks:     domain.ComputeTotal(quantity, unitPrice),
		Metadata:         req.Metadata,
		AuditEventID:     req.AuditEventID,
		CreatedAt:        time.Now().UTC(),
	}

	stored, inserted, err := h.repo.Append(r.Context(), evt)
	if err != nil {
		h.log.Error("append billing event", "id", evt.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "append_failed", "failed to record event")
		return
	}

	status := http.StatusCreated
	if !inserted {
		// Идемпотентный re-insert: по ADR-0010 § 5 событие с тем же id уже есть.
		status = http.StatusOK
	}

	// Audit-emit: tenant_id берём из тела запроса (если withPlatformAuth не
	// получил его из заголовков), entity_id — id только что записанного билинг-события.
	auditsdk.SetTenantID(r.Context(), stored.TenantID)
	auditPayload, _ := json.Marshal(map[string]any{
		"billing_event_id": stored.ID,
		"event_type":       stored.EventType,
		"total_kopecks":    stored.TotalKopecks,
	})
	auditsdk.SetEntity(r.Context(), "billing_event", stored.ID, auditPayload)

	writeJSON(w, status, stored)
}

func (h *EventHandler) getEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	evt, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "billing event not found")
		return
	}
	if err != nil {
		h.log.Error("get billing event", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, evt)
}

func (h *EventHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id is required")
		return
	}
	from, to, err := parsePeriod(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "validation_failed", "limit must be a positive integer")
			return
		}
		if n > maxLimit {
			n = maxLimit
		}
		limit = n
	}

	events, err := h.repo.ListByTenant(r.Context(), tenantID, from, to, limit)
	if err != nil {
		h.log.Error("list billing events", "tenant_id", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": events,
		"count": len(events),
		"from":  from,
		"to":    to,
	})
}

func (h *EventHandler) aggregate(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id is required")
		return
	}
	from, to, err := parsePeriod(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	agg, err := h.repo.AggregateByTenant(r.Context(), tenantID, from, to)
	if err != nil {
		h.log.Error("aggregate billing events", "tenant_id", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, agg)
}

// parsePeriod парсит параметры from/to в формате YYYY-MM-DD.
// Если не указан to — текущее UTC-время; если не указан from — начало текущего месяца.
func parsePeriod(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now

	if raw := r.URL.Query().Get("from"); raw != "" {
		t, err := time.Parse(dateFormatISO, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid 'from' date, expected YYYY-MM-DD")
		}
		from = t.UTC()
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		t, err := time.Parse(dateFormatISO, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid 'to' date, expected YYYY-MM-DD")
		}
		to = t.UTC()
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, errors.New("'to' must be strictly after 'from'")
	}
	return from, to, nil
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

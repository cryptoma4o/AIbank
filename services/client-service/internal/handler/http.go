package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"aibank/client-service/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type repo interface {
	Create(ctx context.Context, c *domain.Client) error
	GetByID(ctx context.Context, tenantID, clientID string) (*domain.Client, error)
	GetByINN(ctx context.Context, tenantID, inn string) (*domain.Client, error)
	AppendEvent(ctx context.Context, e *domain.ClientEvent) error
	ListEvents(ctx context.Context, tenantID, clientID string, limit int) ([]domain.ClientEvent, error)
}

type Handler struct {
	repo repo
}

func NewHandler(r repo) *Handler {
	return &Handler{repo: r}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/v1/clients", func(r chi.Router) {
		r.Post("/", h.createClient)
		r.Get("/by-inn/{inn}", h.getByINN)
		r.Get("/{id}", h.getByID)
		r.Post("/{id}/events", h.appendEvent)
		r.Get("/{id}/events", h.listEvents)
	})

	return r
}

func tenantID(r *http.Request) string {
	return r.Header.Get("X-Tenant-ID")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handler) createClient(w http.ResponseWriter, r *http.Request) {
	var c domain.Client
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if c.INN == "" || c.FullName == "" || c.TenantID == "" {
		writeError(w, http.StatusBadRequest, "inn, full_name and tenant_id are required")
		return
	}
	if err := h.repo.Create(r.Context(), &c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create client")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	tid := tenantID(r)
	if tid == "" {
		writeError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	id := chi.URLParam(r, "id")
	c, err := h.repo.GetByID(r.Context(), tid, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "client not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get client")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) getByINN(w http.ResponseWriter, r *http.Request) {
	tid := tenantID(r)
	if tid == "" {
		writeError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	inn := chi.URLParam(r, "inn")
	c, err := h.repo.GetByINN(r.Context(), tid, inn)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "client not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get client")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) appendEvent(w http.ResponseWriter, r *http.Request) {
	tid := tenantID(r)
	if tid == "" {
		writeError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	clientID := chi.URLParam(r, "id")

	var e domain.ClientEvent
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	e.ClientID = clientID
	e.TenantID = tid

	if e.Category == "" || e.EventType == "" {
		writeError(w, http.StatusBadRequest, "category and event_type are required")
		return
	}

	if err := h.repo.AppendEvent(r.Context(), &e); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to append event")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	tid := tenantID(r)
	if tid == "" {
		writeError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	clientID := chi.URLParam(r, "id")

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}

	events, err := h.repo.ListEvents(r.Context(), tid, clientID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}
	if events == nil {
		events = []domain.ClientEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

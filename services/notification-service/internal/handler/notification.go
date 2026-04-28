// Package handler — HTTP-обработчики notification-service.
//
// Эндпоинты:
//
//	POST /v1/notifications       — создаёт + отправляет уведомление
//	GET  /v1/notifications       — listing с tenant_id фильтром
//	GET  /v1/notifications/{id}  — единичная запись
package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aibank/platform/services/notification-service/internal/domain"
	"github.com/aibank/platform/services/notification-service/internal/sender"
	"github.com/aibank/platform/services/notification-service/internal/templates"
)

// Handler — HTTP-обработчик.
//
// Зависимости:
//   - repo — store.NotificationRepository (Postgres или in-memory для тестов)
//   - tpl  — templates.Registry с зарегистрированными шаблонами
//   - senders — мап канал→Sender; для каждого RecipientType должен быть
//     зарегистрирован Sender, иначе POST вернёт 400.
type Handler struct {
	repo    domain.NotificationRepository
	tpl     *templates.Registry
	senders map[domain.RecipientType]sender.Sender
	log     *slog.Logger
}

// New — конструктор Handler.
func New(repo domain.NotificationRepository, tpl *templates.Registry,
	senders map[domain.RecipientType]sender.Sender, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{repo: repo, tpl: tpl, senders: senders, log: log}
}

// Routes — chi-роутер с эндпоинтами.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/notifications", h.Create)
	r.Get("/notifications", h.List)
	r.Get("/notifications/{id}", h.Get)
	return r
}

// createRequest — тело POST /v1/notifications.
type createRequest struct {
	TenantID      string            `json:"tenant_id"`
	RecipientType string            `json:"recipient_type"`
	Recipient     string            `json:"recipient"`
	TemplateID    string            `json:"template_id"`
	Vars          map[string]string `json:"vars"`
}

func (req createRequest) validate() error {
	if req.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if req.Recipient == "" {
		return errors.New("recipient is required")
	}
	if req.TemplateID == "" {
		return errors.New("template_id is required")
	}
	if !domain.RecipientType(req.RecipientType).IsValid() {
		return errors.New("recipient_type must be email|sms|push")
	}
	return nil
}

// Create — POST /v1/notifications.
//
// Шаги:
//  1. Валидация payload.
//  2. Рендер шаблона.
//  3. Запись в repo (status=queued).
//  4. Sender.Send — синхронно (для MVP; в будущем — Kafka-очередь).
//  5. MarkSent / MarkFailed по результату; status попадает в response.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	rt := domain.RecipientType(req.RecipientType)
	s, ok := h.senders[rt]
	if !ok {
		writeError(w, http.StatusBadRequest, "channel_unavailable",
			"no sender configured for "+req.RecipientType)
		return
	}

	rendered, err := h.tpl.Render(req.TemplateID, req.Vars)
	if err != nil {
		writeError(w, http.StatusBadRequest, "template_render_failed", err.Error())
		return
	}

	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "id_gen_failed", "failed to generate id")
		return
	}
	n := &domain.Notification{
		ID:            id,
		TenantID:      req.TenantID,
		RecipientType: rt,
		Recipient:     req.Recipient,
		TemplateID:    req.TemplateID,
		Vars:          req.Vars,
		Status:        domain.StatusQueued,
		CreatedAt:     time.Now().UTC(),
	}
	if err := h.repo.Create(r.Context(), n); err != nil {
		h.log.Error("create notification", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed",
			"failed to persist notification")
		return
	}

	sendErr := s.Send(r.Context(), n, sender.Rendered{Subject: rendered.Subject, Body: rendered.Body})
	if sendErr != nil {
		_ = h.repo.MarkFailed(context.Background(), id, truncate(sendErr.Error()))
		n.Status = domain.StatusFailed
		n.Error = sendErr.Error()
		h.log.Warn("send failed", "id", id, "err", sendErr)
	} else {
		_ = h.repo.MarkSent(context.Background(), id)
		now := time.Now().UTC()
		n.Status = domain.StatusSent
		n.SentAt = &now
		n.AttemptCount = 1
	}

	writeJSON(w, http.StatusCreated, n)
}

// List — GET /v1/notifications?tenant_id=...&limit=...
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"tenant_id query param is required")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := h.repo.ListByTenant(r.Context(), tenantID, limit)
	if err != nil {
		h.log.Error("list notifications", "tenant", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "list_failed", "failed to list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "count": len(items),
	})
}

// Get — GET /v1/notifications/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	n, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "notification not found")
		return
	}
	if err != nil {
		h.log.Error("get notification", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

// newID — компактный random hex (16 байт = 32 символа).
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ntf_" + hex.EncodeToString(b), nil
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

func truncate(s string) string {
	const max = 1024
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

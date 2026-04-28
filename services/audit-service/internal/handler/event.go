package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/aibank/platform/services/audit-service/internal/domain"
)

// EventSigner — узкий интерфейс для подписания audit-event-digest'а.
//
// Реализации:
//   - packages/signature/ed25519.Signer (Pre-MVP)
//   - адаптер вокруг packages/signature.SignatureProvider (после получения СКЗИ)
//
// nil-signer допустим — handler работает в legacy-режиме без подписи
// (backwards compat для existing producers).
type EventSigner interface {
	Sign(digest []byte) ([]byte, error)
	KeyID() string
}

// SignerAlgorithm возвращает значение для audit.signature_algorithm колонки.
// Реализуется ed25519.Signer (через переменную Algorithm) и внешними signer'ами.
type SignerAlgorithm interface {
	Algorithm() string
}

type EventHandler struct {
	repo      domain.AuditEventRepository
	log       *slog.Logger
	signer    EventSigner
	algorithm string
}

// NewEventHandler — без подписи. Backwards-compatible конструктор.
func NewEventHandler(repo domain.AuditEventRepository, log *slog.Logger) *EventHandler {
	return &EventHandler{repo: repo, log: log}
}

// NewEventHandlerWithSigner — handler с криптографическим signing.
// Если signer == nil — поведение совпадает с NewEventHandler.
// algorithm — значение из ed25519.Algorithm ("ed25519") или эквивалент
// для других signer'ов; влияет на колонку audit.signature_algorithm.
func NewEventHandlerWithSigner(
	repo domain.AuditEventRepository,
	log *slog.Logger,
	signer EventSigner,
	algorithm string,
) *EventHandler {
	return &EventHandler{repo: repo, log: log, signer: signer, algorithm: algorithm}
}

func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.RecordEvent) // write-only
	r.Get("/", h.ListEvents)   // read with filters
	return r
}

type recordEventRequest struct {
	TenantID   string          `json:"tenant_id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	EventType  string          `json:"event_type"`
	ActorID    string          `json:"actor_id"`
	ActorType  domain.ActorType `json:"actor_type"`
	Payload    json.RawMessage  `json:"payload"`
}

func (req recordEventRequest) validate() error {
	if req.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if req.EntityType == "" {
		return errors.New("entity_type is required")
	}
	if req.EntityID == "" {
		return errors.New("entity_id is required")
	}
	if req.EventType == "" {
		return errors.New("event_type is required")
	}
	if req.ActorID == "" {
		return errors.New("actor_id is required")
	}
	switch req.ActorType {
	case domain.ActorTypeUser, domain.ActorTypeSystem, domain.ActorTypeAIAgent:
	default:
		return errors.New("actor_type must be user, system, or ai_agent")
	}
	return nil
}

// RecordEvent добавляет событие в append-only журнал.
//
// Каждое событие связано в hash-цепочку с предыдущим (по tenant_id) для
// tamper-evidence: подмена или удаление события приведёт к разрыву цепочки.
// Append-only enforcement также на уровне БД (триггер prevent_audit_modification).
func (h *EventHandler) RecordEvent(w http.ResponseWriter, r *http.Request) {
	var req recordEventRequest
	// Limit payload to 256KB — события audit log не должны быть огромными.
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	previousHash, err := h.repo.LatestHash(r.Context(), req.TenantID)
	if err != nil {
		h.log.Error("get latest hash", "tenant", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	event := &domain.AuditEvent{
		ID:         "evt_" + uuid.NewString(),
		TenantID:   req.TenantID,
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		EventType:  req.EventType,
		ActorID:    req.ActorID,
		ActorType:  req.ActorType,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}
	event.Hash = event.ComputeHash(previousHash)

	// Опциональная подпись поверх hash-chain. ComputeHash остаётся неизменной
	// даже после signing — signature вне immutable-payload, см. event.go.
	if h.signer != nil {
		digest, err := event.SignedDigest()
		if err != nil {
			h.log.Error("decode hash digest for signing", "tenant", req.TenantID, "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		sig, err := h.signer.Sign(digest)
		if err != nil {
			h.log.Error("sign event", "tenant", req.TenantID, "err", err)
			writeError(w, http.StatusInternalServerError, "sign_failed", "failed to sign event")
			return
		}
		event.Signature = sig
		event.SignatureAlgorithm = h.algorithm
		event.SignerKeyID = h.signer.KeyID()
	}

	if err := h.repo.Append(r.Context(), event); err != nil {
		h.log.Error("append event", "tenant", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "append_failed", "failed to append event")
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

// ListEvents выдаёт события с фильтрами по тенанту/сущности.
// Параметры: tenant_id (required), entity_type, entity_id, limit (default 100, max 1000).
func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(w, http.StatusBadRequest, "validation_failed", "limit must be 1..1000")
			return
		}
		limit = parsed
	}

	events, err := h.repo.List(r.Context(), tenantID, entityType, entityID, limit)
	if err != nil {
		h.log.Error("list events", "tenant", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events, "count": len(events)})
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

// Package handler — HTTP-фасад abs-connector'а.
//
// Реализует POST /v1/commands и GET /v1/commands/{idempotency_key}.
// Согласно ADR-0006 connector — это transparent router + idempotency layer:
// бизнес-логики команд здесь нет, маршрутизация идёт по tenant_id через
// AdapterRegistry, дедупликация — через IdempotencyStore.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aibank/abs-connector/internal/clients"
	"aibank/abs-connector/internal/dedupstore"
	"aibank/abs-connector/internal/domain"
)

// Handler — HTTP-фасад. Все зависимости — interface'ы, чтобы тесты могли
// подменить их мок-реализациями (registry — concrete struct, потому что
// у него нет внешнего state'а).
type Handler struct {
	registry  *domain.AdapterRegistry
	store     domain.IdempotencyStore
	client    *clients.AdapterClient
	cacheTTL  time.Duration
	log       *slog.Logger
	readReady func(ctx context.Context) error
}

// Config описывает обязательные зависимости. readReady — опциональный
// hook для /ready: возвращает nil, если все downstream-системы доступны.
type Config struct {
	Registry  *domain.AdapterRegistry
	Store     domain.IdempotencyStore
	Client    *clients.AdapterClient
	CacheTTL  time.Duration
	Logger    *slog.Logger
	ReadReady func(ctx context.Context) error
}

func New(cfg Config) *Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = dedupstore.DefaultTTL
	}
	if cfg.ReadReady == nil {
		cfg.ReadReady = func(context.Context) error { return nil }
	}
	return &Handler{
		registry:  cfg.Registry,
		store:     cfg.Store,
		client:    cfg.Client,
		cacheTTL:  cfg.CacheTTL,
		log:       cfg.Logger,
		readReady: cfg.ReadReady,
	}
}

// Router возвращает chi.Router с провязанными ручками и middleware.
// Используется как из main.go, так и из тестов через httptest.NewServer.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(45 * time.Second))

	r.Get("/health", h.health)
	r.Get("/ready", h.ready)
	r.Get("/healthz", h.health) // совместимость со старым клиентом

	r.Post("/v1/commands", h.executeCommand)
	r.Get("/v1/commands/{idempotency_key}", h.getCommand)
	return r
}

// --- /health, /ready ---

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.readReady(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- POST /v1/commands ---

type executeRequest struct {
	IdempotencyKey string                 `json:"idempotency_key"`
	TenantID       string                 `json:"tenant_id"`
	Command        domain.CommandType     `json:"command"`
	Payload        json.RawMessage        `json:"payload"`
	Metadata       domain.CommandMetadata `json:"metadata"`
}

func (req executeRequest) validate() error {
	if req.IdempotencyKey == "" {
		return errors.New("idempotency_key is required")
	}
	if _, err := uuid.Parse(req.IdempotencyKey); err != nil {
		return errors.New("idempotency_key must be valid UUID")
	}
	if err := domain.ValidateTenantID(req.TenantID); err != nil {
		return err
	}
	switch req.Command {
	case domain.CmdOpenAccount, domain.CmdCloseAccount,
		domain.CmdGetAccountInfo, domain.CmdCreateClient:
	case "":
		return errors.New("command is required")
	default:
		// Не блокируем: канон может вырасти быстрее, чем connector,
		// но логируем — это поможет отлову незарегистрированных команд.
		// Возвращаем 400 для безопасности: пусть caller пройдёт code review.
		return errors.New("unknown command: " + string(req.Command))
	}
	return nil
}

func (h *Handler) executeCommand(w http.ResponseWriter, r *http.Request) {
	var req executeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	ctx := r.Context()

	// Шаг 1: проверка кеша.
	if cached, ok, err := h.store.Get(ctx, req.TenantID, req.IdempotencyKey); err != nil {
		// Транспортная ошибка кеша — не фатальна для клиента: продолжаем
		// в адаптер, но логируем — alerting должен поднять Redis.
		h.log.Warn("idempotency store get failed",
			"tenant_id", req.TenantID,
			"idempotency_key", req.IdempotencyKey,
			"err", err,
		)
	} else if ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// Шаг 2: lookup адаптера.
	entry, err := h.registry.Lookup(req.TenantID)
	if err != nil {
		if errors.Is(err, domain.ErrTenantNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "adapter_not_configured",
				"tenant has no abs adapter configured")
			return
		}
		h.log.Error("registry lookup failed", "tenant_id", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "registry lookup failed")
		return
	}

	// Шаг 3: вызов адаптера.
	cmd := domain.CanonicalCommand{
		IdempotencyKey: req.IdempotencyKey,
		TenantID:       req.TenantID,
		Command:        req.Command,
		Payload:        req.Payload,
		Metadata:       req.Metadata,
		CreatedAt:      time.Now().UTC(),
	}
	resp, err := h.client.Execute(ctx, clients.ExecuteParams{Entry: entry, Command: cmd})
	if err != nil {
		h.log.Error("adapter call failed",
			"tenant_id", req.TenantID,
			"adapter", entry.AdapterName,
			"adapter_version", entry.Version,
			"err", err,
		)
		writeError(w, http.StatusBadGateway, "adapter_unreachable", err.Error())
		return
	}

	// Шаг 4: сохраняем результат в idempotency-store (24h TTL по умолчанию).
	// Контекст store-вызова не привязываем к request-context'у: мы хотим,
	// чтобы запись в кеш произошла даже если client закрыл соединение.
	storeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if perr := h.store.Put(storeCtx, req.TenantID, req.IdempotencyKey, resp, h.cacheTTL); perr != nil {
		h.log.Warn("idempotency store put failed",
			"tenant_id", req.TenantID,
			"idempotency_key", req.IdempotencyKey,
			"err", perr,
		)
		// Не возвращаем ошибку клиенту: сама команда выполнена адаптером.
	}

	status := http.StatusOK
	if !resp.Success {
		// Пишем 200 с success=false: семантически операция «состоялась
		// и вернула ошибку домена» — это не транспортный 5xx. Caller-Temporal
		// разбирает по полю success.
		status = http.StatusOK
	}
	writeJSON(w, status, resp)
}

// --- GET /v1/commands/{idempotency_key} ---

func (h *Handler) getCommand(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "idempotency_key")
	if _, err := uuid.Parse(key); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", "idempotency_key must be valid UUID")
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	if err := domain.ValidateTenantID(tenantID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	cached, ok, err := h.store.Get(r.Context(), tenantID, key)
	if err != nil {
		h.log.Error("idempotency store get failed",
			"tenant_id", tenantID,
			"idempotency_key", key,
			"err", err,
		)
		writeError(w, http.StatusInternalServerError, "internal_error", "idempotency store error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no cached response for idempotency_key")
		return
	}
	writeJSON(w, http.StatusOK, cached)
}

// --- helpers ---

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

// Package handler — HTTP-слой client-service.
//
// Маршруты следуют доменной модели КУС: клиента создаёт orchestrator после
// принятия Application, далее меняется только status (PATCH) и накапливается
// неизменяемая история. Полнотельный Update сознательно НЕ выставлен —
// доменный поток изменений идёт через append-only ClientHistory.
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
	"github.com/google/uuid"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/client-service/internal/domain"
)

// historyAppender — узкий интерфейс для тестируемости (in-memory stub в тестах).
type historyAppender interface {
	Append(ctx context.Context, h *domain.ClientHistory, tenantID string) error
	ListByClient(ctx context.Context, tenantID, clientID string, limit, offset int) ([]*domain.ClientHistory, error)
}

// ClientHandler связывает HTTP-уровень с репозиториями.
type ClientHandler struct {
	clients     domain.ClientRepository
	history     historyAppender
	auditClient *auditsdk.Client
	log         *slog.Logger
}

// NewClientHandler — конструктор; auditClient может быть nil (best-effort
// per ADR-0010).
func NewClientHandler(
	clients domain.ClientRepository,
	history historyAppender,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *ClientHandler {
	return &ClientHandler{clients: clients, history: history, auditClient: auditClient, log: log}
}

// withPlatformAuth выставляет AuthInfo из X-Tenant-ID/X-Actor-ID для emit middleware.
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

func (h *ClientHandler) resolveCreate(_ *http.Request) (string, string, json.RawMessage, bool) {
	return "", "", nil, false
}
func (h *ClientHandler) resolveStatusChange(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"client_id": id, "action": "status_changed"})
	return "client", id, payload, true
}

// ErrNotFound — единая sentinel-ошибка из репозиториев. Дублируется здесь,
// чтобы handler не зависел от конкретного слоя репозитория в тестах.
var ErrNotFound = errors.New("not found")

// Routes возвращает chi-роутер, прикрепляемый к /v1/clients.
func (h *ClientHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "client.created",
			h.resolveCreate, h.log)).Post("/", h.CreateClient)
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "client.status_changed",
			h.resolveStatusChange, h.log)).Patch("/{id}/status", h.UpdateStatus)
	} else {
		r.Post("/", h.CreateClient)
		r.Patch("/{id}/status", h.UpdateStatus)
	}
	r.Get("/", h.ListClients)
	r.Get("/{id}", h.GetClient)
	r.Get("/{id}/history", h.ListHistory)
	r.Post("/{id}/history", h.AppendHistory)
	return r
}

// --- requests ---------------------------------------------------------------

type createClientRequest struct {
	TenantID      string              `json:"tenant_id"`
	ApplicantID   string              `json:"applicant_id"`
	LegalEntityID string              `json:"legal_entity_id"`
	Status        domain.ClientStatus `json:"status"`
	RiskCategory  domain.RiskCategory `json:"risk_category"`
}

func (req createClientRequest) validate() error {
	if req.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if req.ApplicantID == "" {
		return errors.New("applicant_id is required")
	}
	if req.LegalEntityID == "" {
		return errors.New("legal_entity_id is required")
	}
	if !req.Status.IsValid() {
		return errors.New("status must be onboarding|active|suspended|archived")
	}
	if !req.RiskCategory.IsValid() {
		return errors.New("risk_category must be LOW|MEDIUM|HIGH")
	}
	return nil
}

type updateStatusRequest struct {
	Status domain.ClientStatus `json:"status"`
	Reason string              `json:"reason"`
}

type appendHistoryRequest struct {
	EventType string `json:"event_type"`
	Summary   string `json:"summary"`
	Source    string `json:"source"`
}

// --- handlers ---------------------------------------------------------------

// CreateClient: POST /v1/clients.
// Тело — createClientRequest. Возвращает созданную карточку с присвоенным id.
func (h *ClientHandler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req createClientRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	c := &domain.Client{
		ID:            "cli_" + uuid.NewString(),
		TenantID:      req.TenantID,
		ApplicantID:   req.ApplicantID,
		LegalEntityID: req.LegalEntityID,
		Status:        req.Status,
		RiskCategory:  req.RiskCategory,
	}
	if err := h.clients.Create(r.Context(), c); err != nil {
		h.log.Error("create client", "tenant", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create client")
		return
	}

	auditsdk.SetTenantID(r.Context(), c.TenantID)
	pl, _ := json.Marshal(map[string]any{"client_id": c.ID, "status": c.Status, "risk_category": c.RiskCategory})
	auditsdk.SetEntity(r.Context(), "client", c.ID, pl)

	writeJSON(w, http.StatusCreated, c)
}

// GetClient: GET /v1/clients/{id}?tenant_id=X.
func (h *ClientHandler) GetClient(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")
	c, err := h.clients.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) || isRepoNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "client not found")
		return
	}
	if err != nil {
		h.log.Error("get client", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ListClients: GET /v1/clients?tenant_id=X&limit=&offset=.
func (h *ClientHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	items, err := h.clients.ListByTenant(r.Context(), tenantID, limit, offset)
	if err != nil {
		h.log.Error("list clients", "tenant", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "count": len(items),
		"limit": limit, "offset": offset,
	})
}

// UpdateStatus: PATCH /v1/clients/{id}/status?tenant_id=X.
//
// Помимо смены статуса записывает событие в client_history (append-only),
// чтобы не потерять причину перевода. Источник — сама запись `status` (без
// внешнего audit_event_id), его на этом уровне нет.
func (h *ClientHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")

	var req updateStatusRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !req.Status.IsValid() {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"status must be onboarding|active|suspended|archived")
		return
	}

	if err := h.clients.UpdateStatus(r.Context(), tenantID, id, req.Status); err != nil {
		if errors.Is(err, ErrNotFound) || isRepoNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "client not found")
			return
		}
		h.log.Error("update status", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// Логируем переход в неизменяемую историю. Ошибка тут не должна откатывать
	// сам status — это две независимые операции; пишем warn и идём дальше.
	hist := &domain.ClientHistory{
		ID:        "chi_" + uuid.NewString(),
		ClientID:  id,
		EventType: "status.changed",
		Summary:   "status -> " + string(req.Status) + ": " + req.Reason,
		Source:    "patch:/v1/clients/" + id + "/status",
		CreatedAt: time.Now().UTC(),
	}
	if err := h.history.Append(r.Context(), hist, tenantID); err != nil {
		h.log.Warn("append history after status change failed", "id", id, "err", err)
	}

	auditsdk.SetTenantID(r.Context(), tenantID)
	pl, _ := json.Marshal(map[string]any{"client_id": id, "new_status": req.Status, "reason": req.Reason})
	auditsdk.SetEntity(r.Context(), "client", id, pl)

	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "status": req.Status,
	})
}

// ListHistory: GET /v1/clients/{id}/history?tenant_id=X&limit=&offset=.
func (h *ClientHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	items, err := h.history.ListByClient(r.Context(), tenantID, id, limit, offset)
	if err != nil {
		h.log.Error("list history", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "count": len(items),
		"limit": limit, "offset": offset,
	})
}

// AppendHistory: POST /v1/clients/{id}/history?tenant_id=X.
// Используется orchestrator'ом для регистрации событий (новый документ,
// решение, апдейт риск-метки и пр.).
func (h *ClientHandler) AppendHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")

	var req appendHistoryRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.EventType == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "event_type is required")
		return
	}
	if req.Summary == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "summary is required")
		return
	}

	hist := &domain.ClientHistory{
		ID:        "chi_" + uuid.NewString(),
		ClientID:  id,
		EventType: req.EventType,
		Summary:   req.Summary,
		Source:    req.Source,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.history.Append(r.Context(), hist, tenantID); err != nil {
		h.log.Error("append history", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "append_failed", "failed to append history")
		return
	}
	writeJSON(w, http.StatusCreated, hist)
}

// --- helpers ----------------------------------------------------------------

const (
	defaultLimit = 50
	maxLimit     = 200
)

func parsePagination(r *http.Request) (int, int, error) {
	limit := defaultLimit
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			return 0, 0, errors.New("limit must be 1..200")
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, 0, errors.New("offset must be >= 0")
		}
		offset = n
	}
	return limit, offset, nil
}

// isRepoNotFound — мостик между handler и repository-слоем для NotFound,
// чтобы не тащить импорт repository в тестах с in-memory stub'ами.
func isRepoNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "not found"
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

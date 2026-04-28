// Package handler — HTTP-слой ubo-service.
//
// Поток: orchestrator вызывает agent-ubo-tracing → получает UBOResult →
// шлёт его в POST /v1/ubo-graphs. Сервис материализует snapshot новой
// версией. UI/комплаенс читают latest и историю.
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

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/ubo-service/internal/domain"
)

// GraphHandler связывает HTTP с domain.UBOGraphRepository.
type GraphHandler struct {
	repo        domain.UBOGraphRepository
	auditClient *auditsdk.Client
	log         *slog.Logger
}

// NewGraphHandler — конструктор; auditClient может быть nil (best-effort
// per ADR-0010).
func NewGraphHandler(repo domain.UBOGraphRepository, auditClient *auditsdk.Client, log *slog.Logger) *GraphHandler {
	return &GraphHandler{repo: repo, auditClient: auditClient, log: log}
}

// ErrNotFound — sentinel для маппинга в 404.
var ErrNotFound = errors.New("not found")

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

func (h *GraphHandler) resolveCreate(_ *http.Request) (string, string, json.RawMessage, bool) {
	return "", "", nil, false
}

// Routes возвращает chi-роутер для /v1/ubo-graphs.
func (h *GraphHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "ubo_graph.created",
			h.resolveCreate, h.log)).Post("/", h.CreateGraph)
	} else {
		r.Post("/", h.CreateGraph)
	}
	r.Get("/latest", h.GetLatest)
	r.Get("/", h.ListVersions)
	r.Get("/{id}", h.GetByID)
	return r
}

// --- requests ---------------------------------------------------------------

// createGraphRequest повторяет shape UBOResult из agent-ubo-tracing
// (ai/agent-ubo-tracing/models/schemas.py), плюс tenant_id/legal_entity_id/
// computed_by, которые проставляет orchestrator.
type createGraphRequest struct {
	TenantID            string          `json:"tenant_id"`
	LegalEntityID       string          `json:"legal_entity_id"`
	Nodes               json.RawMessage `json:"nodes"`
	Edges               json.RawMessage `json:"edges"`
	UBOs                json.RawMessage `json:"ubos"`
	Confidence          float64         `json:"confidence"`
	UnresolvedBranches  json.RawMessage `json:"unresolved_branches"`
	ComputedBy          string          `json:"computed_by"`
}

func (req createGraphRequest) validate() error {
	if req.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if req.LegalEntityID == "" {
		return errors.New("legal_entity_id is required")
	}
	if req.ComputedBy == "" {
		return errors.New("computed_by is required (e.g. 'agent-ubo-tracing v1.3')")
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		return errors.New("confidence must be in [0,1]")
	}
	// JSONB-payload'ы валидируем поверхностно: должны быть валидным JSON.
	for name, raw := range map[string]json.RawMessage{
		"nodes": req.Nodes, "edges": req.Edges,
		"ubos": req.UBOs, "unresolved_branches": req.UnresolvedBranches,
	} {
		if len(raw) == 0 {
			continue
		}
		var probe any
		if err := json.Unmarshal(raw, &probe); err != nil {
			return errors.New(name + " is not valid JSON: " + err.Error())
		}
	}
	return nil
}

// --- handlers ---------------------------------------------------------------

// CreateGraph: POST /v1/ubo-graphs.
// Идемпотентность не предполагается: каждый POST — новая версия snapshot'а.
func (h *GraphHandler) CreateGraph(w http.ResponseWriter, r *http.Request) {
	var req createGraphRequest
	// До 2MB — графы по нашим оценкам редко больше 200KB, держим запас.
	if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	g := &domain.UBOGraph{
		ID:                 "ubg_" + uuid.NewString(),
		TenantID:           req.TenantID,
		LegalEntityID:      req.LegalEntityID,
		Nodes:              defaultJSON(req.Nodes, "[]"),
		Edges:              defaultJSON(req.Edges, "[]"),
		UBOs:               defaultJSON(req.UBOs, "[]"),
		Confidence:         req.Confidence,
		UnresolvedBranches: defaultJSON(req.UnresolvedBranches, "[]"),
		ComputedAt:         time.Now().UTC(),
		ComputedBy:         req.ComputedBy,
	}
	if err := h.repo.Create(r.Context(), g); err != nil {
		h.log.Error("create graph", "tenant", req.TenantID, "le", req.LegalEntityID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create ubo graph")
		return
	}

	auditsdk.SetTenantID(r.Context(), g.TenantID)
	pl, _ := json.Marshal(map[string]any{
		"ubo_graph_id":    g.ID,
		"legal_entity_id": g.LegalEntityID,
		"version":         g.Version,
		"confidence":      g.Confidence,
	})
	auditsdk.SetEntity(r.Context(), "ubo_graph", g.ID, pl)

	writeJSON(w, http.StatusCreated, g)
}

// GetLatest: GET /v1/ubo-graphs/latest?tenant_id=X&legal_entity_id=Y.
func (h *GraphHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	tenantID, leID, ok := requireTenantAndLegalEntity(w, r)
	if !ok {
		return
	}
	g, err := h.repo.GetLatestByLegalEntity(r.Context(), tenantID, leID)
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "no ubo graph for legal_entity")
		return
	}
	if err != nil {
		h.log.Error("get latest graph", "tenant", tenantID, "le", leID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

// ListVersions: GET /v1/ubo-graphs?tenant_id=X&legal_entity_id=Y&limit=&offset=.
func (h *GraphHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, leID, ok := requireTenantAndLegalEntity(w, r)
	if !ok {
		return
	}
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	items, err := h.repo.ListByLegalEntity(r.Context(), tenantID, leID, limit, offset)
	if err != nil {
		h.log.Error("list graphs", "tenant", tenantID, "le", leID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "count": len(items),
		"limit": limit, "offset": offset,
	})
}

// GetByID: GET /v1/ubo-graphs/{id}?tenant_id=X.
func (h *GraphHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")
	g, err := h.repo.GetByID(r.Context(), tenantID, id)
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, "not_found", "graph not found")
		return
	}
	if err != nil {
		h.log.Error("get graph", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, g)
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

func requireTenantAndLegalEntity(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenantID := r.URL.Query().Get("tenant_id")
	leID := r.URL.Query().Get("legal_entity_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return "", "", false
	}
	if leID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "legal_entity_id query param is required")
		return "", "", false
	}
	return tenantID, leID, true
}

func defaultJSON(raw json.RawMessage, def string) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(def)
	}
	return raw
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
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

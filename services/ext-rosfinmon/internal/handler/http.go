package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aibank/ext-rosfinmon/internal/cache"
	"aibank/ext-rosfinmon/internal/domain"
	"aibank/ext-rosfinmon/internal/provider"
)

// Handler — HTTP-обработчик ext-rosfinmon (115-ФЗ скрининг).
//
// Эндпоинты:
//
//	POST /v1/rosfinmon/screen      — проверка субъекта по перечню 115-ФЗ
//	GET  /v1/rosfinmon/list-info   — метаданные перечня (last_updated, count)
//	GET  /healthz                  — liveness
type Handler struct {
	cache    cache.Cache
	provider provider.Provider
}

// NewHandler собирает роутер с подключённым cache-слоем и provider'ом.
//
// Если provider не передан — используется SyntheticProvider (обратная совместимость).
func NewHandler(c cache.Cache, p ...provider.Provider) http.Handler {
	h := &Handler{cache: c}
	if len(p) > 0 && p[0] != nil {
		h.provider = p[0]
	} else {
		h.provider = provider.NewSyntheticProvider()
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.healthz)
	r.Post("/v1/rosfinmon/screen", h.screen)
	r.Get("/v1/rosfinmon/list-info", h.listInfo)
	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"ext-rosfinmon"}`))
}

func (h *Handler) screen(w http.ResponseWriter, r *http.Request) {
	var req domain.ScreeningRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cached, err := h.cache.GetScreening(r.Context(), req); err == nil && cached != nil {
		respond(w, http.StatusOK, response{Data: cached, Source: "cache"})
		return
	}

	res, err := h.provider.Screen(r.Context(), req)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	res.RequestID = uuid.New().String()
	_ = h.cache.SetScreening(r.Context(), req, &res)

	respond(w, http.StatusOK, response{Data: res, Source: providerSource(h.provider)})
}

func (h *Handler) listInfo(w http.ResponseWriter, r *http.Request) {
	if cached, err := h.cache.GetSnapshot(r.Context()); err == nil && cached != nil {
		respond(w, http.StatusOK, response{Data: cached, Source: "cache"})
		return
	}
	snap, err := h.provider.GetSnapshot(r.Context())
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	_ = h.cache.SetSnapshot(r.Context(), &snap)
	respond(w, http.StatusOK, response{Data: snap, Source: providerSource(h.provider)})
}

// --- helpers ---

type response struct {
	Data   interface{} `json:"data"`
	Source string      `json:"source"`
}

func providerSource(p provider.Provider) string {
	switch p.Name() {
	case "live":
		return "live"
	default:
		return "stub"
	}
}

func writeProviderError(w http.ResponseWriter, name string, err error) {
	if errors.Is(err, provider.ErrNotImplemented) {
		respondError(w, http.StatusServiceUnavailable,
			"провайдер '"+name+"' пока не реализован: "+err.Error())
		return
	}
	respondError(w, http.StatusBadGateway,
		"провайдер '"+name+"' вернул ошибку: "+err.Error())
}

func respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}

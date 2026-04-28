package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/ext-fssp/internal/cache"
	"aibank/ext-fssp/internal/domain"
	"aibank/ext-fssp/internal/provider"
)

// Handler — HTTP-обработчик ext-fssp.
//
// Эндпоинты:
//
//	GET /v1/fssp/by-inn/{inn}                                  — ЮЛ/ИП
//	GET /v1/fssp/by-person?full_name=...&birth_date=YYYY-MM-DD — ФЛ
//	GET /healthz                                               — liveness
type Handler struct {
	cache    cache.Cache
	provider provider.Provider
}

// NewHandler собирает роутер.
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
	r.Get("/v1/fssp/by-inn/{inn}", h.byINN)
	r.Get("/v1/fssp/by-person", h.byPerson)
	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"ext-fssp"}`))
}

func (h *Handler) byINN(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")
	if err := domain.ValidateINN(inn); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cached, err := h.cache.GetByINN(r.Context(), inn); err == nil && cached != nil {
		respond(w, http.StatusOK, response{Data: cached, Source: "cache"})
		return
	}

	res, err := h.provider.GetByINN(r.Context(), inn)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	_ = h.cache.SetByINN(r.Context(), inn, &res)
	respond(w, http.StatusOK, response{Data: res, Source: providerSource(h.provider)})
}

func (h *Handler) byPerson(w http.ResponseWriter, r *http.Request) {
	fullName := r.URL.Query().Get("full_name")
	birthDate := r.URL.Query().Get("birth_date")
	if err := domain.ValidatePersonQuery(fullName, birthDate); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cached, err := h.cache.GetByPerson(r.Context(), fullName, birthDate); err == nil && cached != nil {
		respond(w, http.StatusOK, response{Data: cached, Source: "cache"})
		return
	}

	res, err := h.provider.GetByPerson(r.Context(), fullName, birthDate)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	_ = h.cache.SetByPerson(r.Context(), fullName, birthDate, &res)
	respond(w, http.StatusOK, response{Data: res, Source: providerSource(h.provider)})
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

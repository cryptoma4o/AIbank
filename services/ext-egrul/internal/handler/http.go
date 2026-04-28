package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/ext-egrul/internal/cache"
	"aibank/ext-egrul/internal/domain"
	"aibank/ext-egrul/internal/provider"
)

// Handler — HTTP-обработчик ext-egrul.
//
// Экспортируемые роуты:
//
//	GET /v1/egrul/by-inn/{inn}       — карточка ЮЛ/ИП по ИНН
//	GET /v1/egrul/by-ogrn/{ogrn}     — карточка по ОГРН/ОГРНИП
//	GET /v1/egrul/founders/{inn}     — учредители ЮЛ (ИП — пустой список)
//	GET /healthz                     — liveness
type Handler struct {
	cache    cache.Cache
	provider provider.Provider
}

// NewHandler собирает роутер с подключённым cache-слоем и provider'ом.
//
// provider может быть синтетическим (для dev/CI) или live (через factory.BuildProvider).
// Если передан nil — используется SyntheticProvider (обратная совместимость для тестов).
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
	r.Get("/v1/egrul/by-inn/{inn}", h.byINN)
	r.Get("/v1/egrul/by-ogrn/{ogrn}", h.byOGRN)
	r.Get("/v1/egrul/founders/{inn}", h.founders)

	// Backward-compat: старый путь, который существовал до hardening.
	r.Get("/v1/egrul/{inn}", h.byINN)

	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"ext-egrul"}`))
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

	rec, err := h.provider.GetByINN(r.Context(), inn)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	_ = h.cache.SetByINN(r.Context(), &rec)
	respond(w, http.StatusOK, response{Data: rec, Source: providerSource(h.provider)})
}

func (h *Handler) byOGRN(w http.ResponseWriter, r *http.Request) {
	ogrn := chi.URLParam(r, "ogrn")
	if err := domain.ValidateOGRN(ogrn); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cached, err := h.cache.GetByOGRN(r.Context(), ogrn); err == nil && cached != nil {
		respond(w, http.StatusOK, response{Data: cached, Source: "cache"})
		return
	}

	rec, err := h.provider.GetByOGRN(r.Context(), ogrn)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	_ = h.cache.SetByOGRN(r.Context(), &rec)
	respond(w, http.StatusOK, response{Data: rec, Source: providerSource(h.provider)})
}

func (h *Handler) founders(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")
	if err := domain.ValidateINN(inn); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if cached, err := h.cache.GetFounders(r.Context(), inn); err == nil && cached != nil {
		respond(w, http.StatusOK, response{
			Data:   domain.FoundersResult{INN: inn, Count: len(cached), Founders: cached},
			Source: "cache",
		})
		return
	}

	founders, err := h.provider.GetFounders(r.Context(), inn)
	if err != nil {
		writeProviderError(w, h.provider.Name(), err)
		return
	}
	if founders == nil {
		founders = []domain.Founder{}
	}
	_ = h.cache.SetFounders(r.Context(), inn, founders)
	respond(w, http.StatusOK, response{
		Data:   domain.FoundersResult{INN: inn, Count: len(founders), Founders: founders},
		Source: providerSource(h.provider),
	})
}

// --- helpers ---

type response struct {
	Data   interface{} `json:"data"`
	Source string      `json:"source"` // "cache" | "stub" | "live"
}

// providerSource мапит имя провайдера в значение поля source ответа.
func providerSource(p provider.Provider) string {
	switch p.Name() {
	case "live":
		return "live"
	default:
		return "stub"
	}
}

// writeProviderError возвращает 503, если провайдер ещё не готов (live-заглушка),
// иначе — 502 с пробросом текста ошибки.
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

// errClientNil — sentinel: cache не сконфигурирован.
var errClientNil = errors.New("cache client is nil")

var _ = errClientNil // reserved for future health-check use

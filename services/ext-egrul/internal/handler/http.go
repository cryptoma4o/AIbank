package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/ext-egrul/internal/cache"
	"aibank/ext-egrul/internal/domain"
)

type Handler struct {
	cache *cache.RedisCache
}

func NewHandler(c *cache.RedisCache) http.Handler {
	h := &Handler{cache: c}
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/healthz", h.healthz)
	r.Get("/v1/egrul/{inn}", h.lookupINN)
	return r
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) lookupINN(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")

	cached, err := h.cache.Get(r.Context(), inn)
	if err == nil && cached != nil {
		respond(w, http.StatusOK, map[string]interface{}{
			"data":   cached,
			"source": "cache",
		})
		return
	}

	rec := stubRecord(inn)
	_ = h.cache.Set(r.Context(), rec)

	respond(w, http.StatusOK, map[string]interface{}{
		"data":   rec,
		"source": "stub",
	})
}

func stubRecord(inn string) *domain.EGRULRecord {
	ogrn := "1" + inn
	if len(inn) >= 9 {
		ogrn = "1" + inn[:9]
	}
	return &domain.EGRULRecord{
		INN:      inn,
		OGRN:     ogrn,
		FullName: "ООО «Заглушка " + inn + "»",
		OKVED:    "62.01",
		Address:  "г. Москва, ул. Тестовая, д. 1",
		CEO:      "Иванов И.И.",
		Status:   "active",
	}
}

func respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

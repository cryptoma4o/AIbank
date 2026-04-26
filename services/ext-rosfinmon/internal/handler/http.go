package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/ext-rosfinmon/internal/store"
)

type Handler struct {
	store *store.MemoryStore
}

func NewHandler(s *store.MemoryStore) http.Handler {
	h := &Handler{store: s}
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/healthz", h.healthz)
	r.Get("/v1/rosfinmon/check/{inn}", h.checkINN)
	r.Post("/v1/rosfinmon/reload", h.reload)
	return r
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) checkINN(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")
	result := h.store.Check(inn)
	respond(w, http.StatusOK, result)
}

func (h *Handler) reload(w http.ResponseWriter, r *http.Request) {
	// Stub: real download from Росфинмониторинг XML feed comes later
	respond(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"loaded": 0,
	})
}

func respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

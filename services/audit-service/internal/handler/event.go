package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type EventHandler struct{}

func NewEventHandler() *EventHandler { return &EventHandler{} }

func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.RecordEvent)   // write-only
	r.Get("/", h.ListEvents)     // read with filters
	return r
}

func (h *EventHandler) RecordEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

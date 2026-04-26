package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type DocumentHandler struct{}

func NewDocumentHandler() *DocumentHandler { return &DocumentHandler{} }

func (h *DocumentHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/applications/{appID}/documents", h.UploadDocument)
	r.Get("/documents/{id}", h.GetDocument)
	r.Get("/applications/{appID}/documents", h.ListDocuments)
	r.Patch("/documents/{id}/status", h.UpdateStatus)
	return r
}

func (h *DocumentHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

func (h *DocumentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

func (h *DocumentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

func (h *DocumentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]string{"error": "not implemented"})
}

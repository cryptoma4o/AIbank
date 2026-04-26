package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"aibank/ext-fssp/internal/domain"
)

func NewHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/healthz", healthz)
	r.Get("/v1/fssp/{inn}", lookupINN)
	return r
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func lookupINN(w http.ResponseWriter, r *http.Request) {
	inn := chi.URLParam(r, "inn")
	// Stub: real ФССП OpenData API integration comes later
	result := domain.FSSPResult{
		INN:         inn,
		HasActive:   false,
		TotalDebt:   0,
		Proceedings: []domain.EnforcementProceeding{},
	}
	respond(w, http.StatusOK, result)
}

func respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

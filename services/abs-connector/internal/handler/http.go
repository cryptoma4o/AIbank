package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"aibank/abs-connector/internal/domain"
	"aibank/abs-connector/internal/router"
)

const (
	dedupKeyPrefix = "abs:dedup:"
	dedupTTL       = 24 * time.Hour
)

type Handler struct {
	router *router.Router
	rdb    *redis.Client
}

func NewHandler(r *router.Router, rdb *redis.Client) *Handler {
	return &Handler{router: r, rdb: rdb}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.healthz)
	r.Post("/v1/commands", h.executeCommand)

	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) executeCommand(w http.ResponseWriter, r *http.Request) {
	var cmd domain.ABSCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cmd.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key is required")
		return
	}

	ctx := r.Context()
	redisKey := dedupKeyPrefix + cmd.IdempotencyKey

	// Check Redis for existing response (idempotency check).
	cached, err := h.rdb.Get(ctx, redisKey).Bytes()
	if err == nil {
		var cachedResp domain.ABSResponse
		if jsonErr := json.Unmarshal(cached, &cachedResp); jsonErr == nil {
			writeJSON(w, http.StatusOK, cachedResp)
			return
		}
	}

	// Route to the appropriate adapter.
	resp, err := h.router.Route(ctx, &cmd)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Store response in Redis with 24h TTL.
	if data, marshalErr := json.Marshal(resp); marshalErr == nil {
		_ = h.rdb.Set(context.Background(), redisKey, data, dedupTTL).Err()
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

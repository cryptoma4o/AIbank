package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aibank/abs-adapter-diasoft/internal/domain"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.healthz)
	r.Post("/v1/execute", h.execute)

	return r
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	var cmd domain.ABSCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, domain.ABSResponse{
			IdempotencyKey: "",
			Success:        false,
			Error:          "invalid request body",
			AdapterUsed:    "abs-adapter-diasoft",
		})
		return
	}

	resp := h.stub(cmd)
	writeJSON(w, http.StatusOK, resp)
}

// stub executes a Diasoft FA# stub and returns a canonical ABSResponse.
func (h *Handler) stub(cmd domain.ABSCommand) domain.ABSResponse {
	base := domain.ABSResponse{
		IdempotencyKey: cmd.IdempotencyKey,
		AdapterUsed:    "abs-adapter-diasoft",
	}

	switch cmd.Command {
	case domain.CmdOpenAccount:
		raw := strings.ReplaceAll(uuid.New().String(), "-", "")
		accountNumber := "40702810DS" + raw[:10]
		base.Success = true
		base.Data = map[string]any{
			"account_number": accountNumber,
			"bik":            "044585219",
		}

	case domain.CmdCreateClient:
		base.Success = true
		base.Data = map[string]any{
			"client_id": "ds_" + uuid.New().String(),
		}

	case domain.CmdGetAccountInfo:
		base.Success = true
		base.Data = map[string]any{
			"balance_kopecks": 0,
			"status":          "active",
			"abs":             "diasoft",
		}

	case domain.CmdCloseAccount:
		base.Success = true
		base.Data = map[string]any{
			"status": "closed",
			"abs":    "diasoft",
		}

	default:
		base.Success = false
		base.Error = "unknown command: " + string(cmd.Command)
	}

	return base
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

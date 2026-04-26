package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aibank/abs-adapter-rs-bank/internal/domain"
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
			AdapterUsed:    "abs-adapter-rs-bank",
		})
		return
	}

	resp := h.stub(cmd)
	writeJSON(w, http.StatusOK, resp)
}

// stub executes an RS-Bank/ЦАБС stub and returns a canonical ABSResponse.
func (h *Handler) stub(cmd domain.ABSCommand) domain.ABSResponse {
	base := domain.ABSResponse{
		IdempotencyKey: cmd.IdempotencyKey,
		AdapterUsed:    "abs-adapter-rs-bank",
	}

	switch cmd.Command {
	case domain.CmdOpenAccount:
		raw := strings.ReplaceAll(uuid.New().String(), "-", "")
		accountNumber := "40702810RS" + raw[:10]
		base.Success = true
		base.Data = map[string]any{
			"account_number": accountNumber,
			"bik":            "044030653",
		}

	case domain.CmdCreateClient:
		base.Success = true
		base.Data = map[string]any{
			"client_id": "rs_" + uuid.New().String(),
		}

	case domain.CmdGetAccountInfo:
		base.Success = true
		base.Data = map[string]any{
			"balance_kopecks": 0,
			"status":          "active",
			"abs":             "rs-bank",
		}

	case domain.CmdCloseAccount:
		base.Success = true
		base.Data = map[string]any{
			"status": "closed",
			"abs":    "rs-bank",
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

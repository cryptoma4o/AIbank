package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aibank/identity-service/internal/auth"
)

type Handler struct {
	otpStore   *auth.OTPStore
	esiaClient *auth.ESIAClient
}

func New(otpStore *auth.OTPStore, esiaClient *auth.ESIAClient) *Handler {
	return &Handler{
		otpStore:   otpStore,
		esiaClient: esiaClient,
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", h.healthz)

	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/otp/send", h.otpSend)
		r.Post("/otp/verify", h.otpVerify)
		r.Get("/esia/url", h.esiaURL)
		r.Get("/jwks", h.jwks)
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /healthz
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /v1/auth/otp/send
// Body: {"phone": "+79991234567", "tenant_id": "..."}
func (h *Handler) otpSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone    string `json:"phone"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone is required")
		return
	}

	// Stub: generate OTP without actually sending SMS
	_, err := h.otpStore.Generate(req.Phone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate OTP")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

// POST /v1/auth/otp/verify
// Body: {"phone": "...", "code": "...", "tenant_id": "..."}
func (h *Handler) otpVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone    string `json:"phone"`
		Code     string `json:"code"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Phone == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "phone and code are required")
		return
	}

	if !h.otpStore.Verify(req.Phone, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid or expired OTP")
		return
	}

	sessionID := uuid.New().String()
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id":   sessionID,
		"access_token": "stub-token",
	})
}

// GET /v1/auth/esia/url?tenant_id=&state=
func (h *Handler) esiaURL(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		writeError(w, http.StatusBadRequest, "state is required")
		return
	}

	url := h.esiaClient.AuthURL(state)
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// GET /v1/auth/jwks
func (h *Handler) jwks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keys": []any{}})
}

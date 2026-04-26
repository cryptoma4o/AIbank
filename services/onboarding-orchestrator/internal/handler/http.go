package handler

import (
	"encoding/json"
	"net/http"

	"aibank/onboarding-orchestrator/internal/domain"
	wf "aibank/onboarding-orchestrator/internal/workflow"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
)

const taskQueue = "onboarding"

// Handler holds dependencies for the HTTP layer.
type Handler struct {
	temporal client.Client
}

// New creates a Handler and returns a configured chi router.
func New(tc client.Client) http.Handler {
	h := &Handler{temporal: tc}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", h.Healthz)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/applications", h.StartWorkflow)
		r.Get("/applications/{id}", h.GetStatus)
	})

	return r
}

// Healthz returns 200 OK for liveness probes.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type startRequest struct {
	TenantID string `json:"tenant_id"`
	INN      string `json:"inn"`
	OGRN     string `json:"ogrn"`
}

type startResponse struct {
	ApplicationID string `json:"application_id"`
	WorkflowID    string `json:"workflow_id"`
}

// StartWorkflow starts a new onboarding workflow for the provided application data.
func (h *Handler) StartWorkflow(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	appID := uuid.New().String()
	workflowID := "app-" + appID

	input := domain.OnboardingInput{
		ApplicationID: appID,
		TenantID:      req.TenantID,
		INN:           req.INN,
		OGRN:          req.OGRN,
	}

	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}

	_, err := h.temporal.ExecuteWorkflow(r.Context(), opts, wf.OnboardingWorkflow, input)
	if err != nil {
		http.Error(w, "failed to start workflow: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(startResponse{
		ApplicationID: appID,
		WorkflowID:    workflowID,
	})
}

type statusResponse struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
}

// GetStatus describes an existing onboarding workflow by application ID.
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	workflowID := "app-" + appID

	resp, err := h.temporal.DescribeWorkflowExecution(r.Context(), workflowID, "")
	if err != nil {
		http.Error(w, "workflow not found: "+err.Error(), http.StatusNotFound)
		return
	}

	execInfo := resp.GetWorkflowExecutionInfo()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statusResponse{
		WorkflowID: execInfo.GetExecution().GetWorkflowId(),
		RunID:      execInfo.GetExecution().GetRunId(),
		Status:     execInfo.GetStatus().String(),
	})
}

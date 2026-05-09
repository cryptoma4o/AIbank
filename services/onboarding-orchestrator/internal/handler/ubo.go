package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
)

var validNodeType = map[string]bool{
	"person": true, "legal_entity": true,
}
var validControlBasis = map[string]bool{
	"capital_share": true, "contract": true, "other_decision_right": true,
}

// UBOHandler — этап 5 формы онбординга. Принимает граф владения целиком
// (nodes + edges + chains) и сохраняет в ubo_graphs одной транзакцией.
type UBOHandler struct {
	repo domain.UBOGraphRepository
	log  *slog.Logger
}

func NewUBOHandler(repo domain.UBOGraphRepository, log *slog.Logger) *UBOHandler {
	return &UBOHandler{repo: repo, log: log}
}

func (h *UBOHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

type uboRequest struct {
	TenantID             string                     `json:"tenant_id"`
	ApplicationID        string                     `json:"application_id"`
	LegalEntityID        string                     `json:"legal_entity_id"`
	Nodes                []domain.UBONode           `json:"nodes"`
	Edges                []domain.UBOEdge           `json:"edges"`
	OwnershipChains      []domain.UBOOwnershipChain `json:"ownership_chains,omitempty"`
	NoUBOReason          string                     `json:"no_ubo_reason,omitempty"`
	EIOAsUBOConfirmation bool                       `json:"eio_as_ubo_confirmation"`
	DiagramDocID         string                     `json:"diagram_doc_id,omitempty"`
}

func (req uboRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if strings.TrimSpace(req.ApplicationID) == "" || strings.TrimSpace(req.LegalEntityID) == "" {
		return errors.New("application_id and legal_entity_id are required")
	}
	// Валидация: либо есть UBO-узлы, либо указано обоснование отсутствия.
	hasUBO := false
	for _, n := range req.Nodes {
		if n.IsUBO {
			hasUBO = true
			break
		}
	}
	if !hasUBO && strings.TrimSpace(req.NoUBOReason) == "" && !req.EIOAsUBOConfirmation {
		return errors.New("either a UBO node, no_ubo_reason or eio_as_ubo_confirmation is required (115-FZ)")
	}
	for i, n := range req.Nodes {
		if !validNodeType[n.NodeType] {
			return errors.New("nodes[" + intToStr(i) + "].node_type must be person|legal_entity")
		}
		if n.DirectStake < 0 || n.DirectStake > 100 {
			return errors.New("nodes[" + intToStr(i) + "].direct_stake must be 0..100")
		}
		if n.EffectiveStake < 0 || n.EffectiveStake > 100 {
			return errors.New("nodes[" + intToStr(i) + "].effective_stake must be 0..100")
		}
		if n.ControlBasis != "" && !validControlBasis[n.ControlBasis] {
			return errors.New("nodes[" + intToStr(i) + "].control_basis must be capital_share|contract|other_decision_right")
		}
	}
	for i, e := range req.Edges {
		if e.Stake < 0 || e.Stake > 100 {
			return errors.New("edges[" + intToStr(i) + "].stake must be 0..100")
		}
		if strings.TrimSpace(e.FromNodeID) == "" || strings.TrimSpace(e.ToNodeID) == "" {
			return errors.New("edges[" + intToStr(i) + "]: from_node_id and to_node_id required")
		}
	}
	return nil
}

func (h *UBOHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req uboRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := "ubn_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	now := time.Now().UTC()
	g := &domain.UBOGraph{
		ID:                   id,
		TenantID:             req.TenantID,
		ApplicationID:        req.ApplicationID,
		LegalEntityID:        req.LegalEntityID,
		Nodes:                req.Nodes,
		Edges:                req.Edges,
		OwnershipChains:      req.OwnershipChains,
		NoUBOReason:          req.NoUBOReason,
		EIOAsUBOConfirmation: req.EIOAsUBOConfirmation,
		DiagramDocID:         req.DiagramDocID,
		ComputedAt:           &now,
		CreatedAt:            now,
	}
	if err := h.repo.Upsert(r.Context(), g); err != nil {
		h.log.Error("upsert ubo_graph", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save ubo_graph")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back ubo_graph", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *UBOHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	g, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "ubo_graph not found")
		return
	}
	if err != nil {
		h.log.Error("get ubo_graph", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

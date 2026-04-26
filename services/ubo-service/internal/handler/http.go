package handler

import (
	"encoding/json"
	"net/http"

	"aibank/ubo-service/internal/domain"
	"aibank/ubo-service/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type UBOHandler struct {
	repo *repository.UBORepository
}

func NewUBOHandler(repo *repository.UBORepository) *UBOHandler {
	return &UBOHandler{repo: repo}
}

func (h *UBOHandler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", h.healthz)

	r.Route("/v1/graphs/{app_id}", func(r chi.Router) {
		r.Get("/", h.getGraph)
		r.Post("/nodes", h.addNode)
		r.Post("/edges", h.addEdge)
		r.Get("/stakes/{root_node_id}", h.getStakes)
	})

	return r
}

func (h *UBOHandler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *UBOHandler) tenantID(r *http.Request) string {
	t := r.Header.Get("X-Tenant-ID")
	if t == "" {
		t = "default"
	}
	return t
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /v1/graphs/{app_id}
func (h *UBOHandler) getGraph(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "app_id")
	tenantID := h.tenantID(r)

	graph, err := h.repo.GetGraph(r.Context(), tenantID, appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Mask passport numbers in response
	for i := range graph.Nodes {
		if graph.Nodes[i].Passport != "" {
			graph.Nodes[i].Passport = "****"
		}
	}
	for i := range graph.UBOs {
		if graph.UBOs[i].Passport != "" {
			graph.UBOs[i].Passport = "****"
		}
	}

	writeJSON(w, http.StatusOK, graph)
}

// POST /v1/graphs/{app_id}/nodes
func (h *UBOHandler) addNode(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "app_id")
	tenantID := h.tenantID(r)

	var n domain.UBONode
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	n.AppID = appID
	n.TenantID = tenantID

	if n.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if n.NodeType != domain.NodeTypePerson && n.NodeType != domain.NodeTypeCompany {
		writeError(w, http.StatusBadRequest, "node_type must be 'person' or 'company'")
		return
	}

	if err := h.repo.UpsertNode(r.Context(), &n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	n.Passport = "" // never return raw passport
	writeJSON(w, http.StatusCreated, n)
}

// POST /v1/graphs/{app_id}/edges
func (h *UBOHandler) addEdge(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "app_id")
	tenantID := h.tenantID(r)

	var e domain.UBOEdge
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	e.AppID = appID
	e.TenantID = tenantID

	if e.FromNodeID == "" || e.ToNodeID == "" {
		writeError(w, http.StatusBadRequest, "from_node_id and to_node_id are required")
		return
	}
	if e.DirectStake <= 0 || e.DirectStake > 100 {
		writeError(w, http.StatusBadRequest, "direct_stake must be between 0 and 100")
		return
	}

	if err := h.repo.AddEdge(r.Context(), &e); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, e)
}

// GET /v1/graphs/{app_id}/stakes/{root_node_id}
func (h *UBOHandler) getStakes(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "app_id")
	rootNodeID := chi.URLParam(r, "root_node_id")
	tenantID := h.tenantID(r)

	stakes, err := h.repo.ComputeEffectiveStakes(r.Context(), tenantID, appID, rootNodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type stakeEntry struct {
		NodeID        string  `json:"node_id"`
		EffectiveStake float64 `json:"effective_stake"`
		IsUBO         bool    `json:"is_ubo"`
	}

	entries := make([]stakeEntry, 0, len(stakes))
	for nodeID, stake := range stakes {
		entries = append(entries, stakeEntry{
			NodeID:        nodeID,
			EffectiveStake: stake,
			IsUBO:         stake >= 25.0,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":      appID,
		"root_node_id": rootNodeID,
		"stakes":      entries,
	})
}

package handler

import (
	"encoding/json"
	"net/http"

	"aibank/risk-engine/internal/domain"
	"aibank/risk-engine/internal/scorer"
)

type Handler struct {
	ruleScorer *scorer.RuleScorer
	mlScorer   *scorer.MLScorer
}

func New(rs *scorer.RuleScorer, ml *scorer.MLScorer) *Handler {
	return &Handler{ruleScorer: rs, mlScorer: ml}
}

func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Score(w http.ResponseWriter, r *http.Request) {
	var req domain.ScoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Build facts map from request fields + merge extra Facts
	facts := map[string]any{
		"okved":            req.OKVED,
		"inn":              req.INN,
		"ogrn":             req.OGRN,
		"company_age_years": req.CompanyAge,
	}
	for k, v := range req.Facts {
		facts[k] = v
	}

	// Evaluate declarative rules
	evalResult := h.ruleScorer.Evaluate(facts)

	// Base score: start at 80, apply score adjustment, clamp to [0,100]
	ruleScore := 80 + evalResult.ScoreAdjust
	if ruleScore < 0 {
		ruleScore = 0
	}
	if ruleScore > 100 {
		ruleScore = 100
	}

	// ML score (stub — currently always nil)
	mlScore := h.mlScorer.Score(facts)

	finalScore := ruleScore
	if mlScore != nil {
		// Weighted average: 60% ML, 40% rules
		finalScore = int(float64(*mlScore)*0.6 + float64(ruleScore)*0.4)
		if finalScore < 0 {
			finalScore = 0
		}
		if finalScore > 100 {
			finalScore = 100
		}
	}

	result := domain.ScoreResult{
		ApplicationID: req.ApplicationID,
		Score:         finalScore,
		Blocked:       evalResult.Blocked,
		Flags:         evalResult.Flags,
		RequiredDocs:  evalResult.RequiredDocs,
		FiredRules:    evalResult.FiredRules,
		MLScore:       mlScore,
	}

	// Ensure slices are never null in JSON
	if result.Flags == nil {
		result.Flags = []string{}
	}
	if result.RequiredDocs == nil {
		result.RequiredDocs = []string{}
	}
	if result.FiredRules == nil {
		result.FiredRules = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

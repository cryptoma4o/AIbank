package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	ruleengine "aibank/rule-engine"

	"github.com/google/uuid"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
	"github.com/aibank/platform/services/risk-engine/internal/scorer"
)

// Pipeline orchestrates one full assessment: extract → score → rules → persist.
//
// It is the single source of truth for the recommendation logic spelled out in
// docs/technical-structure.md § 4.2:
//
//   - score < 0.3 AND no high-severity rules → AUTO_APPROVE
//   - 0.3..0.7 OR medium-severity rules     → MANUAL_REVIEW
//   - score > 0.7 OR any blocking rule       → DECLINE_RECOMMENDED
type Pipeline struct {
	extractor   domain.FeatureExtractor
	scorer      scorer.CatBoostScorer
	explainer   scorer.Explainer
	repo        domain.RiskAssessmentRepository
	rulesEngine *ruleengine.Engine // may be nil — pipeline still works without rules
	clock       func() time.Time
	idGen       func() string
}

// Config bundles the dependencies expected by NewPipeline.
type Config struct {
	Extractor domain.FeatureExtractor
	Scorer    scorer.CatBoostScorer
	Explainer scorer.Explainer
	Repo      domain.RiskAssessmentRepository
	Rules     *ruleengine.Engine // nil is allowed (means "no declarative rules")
	Clock     func() time.Time   // optional; defaults to time.Now().UTC()
	IDGen     func() string      // optional; defaults to UUID v4
}

// NewPipeline builds a fully wired pipeline.
func NewPipeline(cfg Config) *Pipeline {
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = func() string { return uuid.NewString() }
	}
	return &Pipeline{
		extractor:   cfg.Extractor,
		scorer:      cfg.Scorer,
		explainer:   cfg.Explainer,
		repo:        cfg.Repo,
		rulesEngine: cfg.Rules,
		clock:       clock,
		idGen:       idGen,
	}
}

// Assess runs one scoring pass end-to-end and persists the result.
//
// inputData is the raw fact dictionary delivered by the caller (orchestrator
// or BFF). It feeds both the FeatureExtractor and the rule engine.
func (p *Pipeline) Assess(
	ctx context.Context, tenantID, applicationID string, inputData map[string]any,
) (*domain.RiskAssessment, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if applicationID == "" {
		return nil, fmt.Errorf("application_id is required")
	}

	features, err := p.extractor.Extract(inputData)
	if err != nil {
		return nil, fmt.Errorf("extract features: %w", err)
	}

	score, factors, err := p.scorer.Score(features)
	if err != nil {
		return nil, fmt.Errorf("score features: %w", err)
	}

	rulesTriggered := p.runRules(features, inputData)
	explanation := p.explainer.Explain(score, factors, rulesTriggered)
	recommendation := DecideRecommendation(score, rulesTriggered)
	now := p.clock()

	model := p.scorer.ModelInfo()
	model.ComputedAt = now

	assessment := &domain.RiskAssessment{
		ID:               p.idGen(),
		TenantID:         tenantID,
		ApplicationID:    applicationID,
		Score:            score,
		Category:         domain.CategoryFromScore(score),
		Model:            model,
		Factors:          factors,
		RulesTriggered:   rulesTriggered,
		ScreeningResults: emptyScreening(now), // populated by ext-rosfinmon / ext-fssp later
		Explanation:      explanation,
		Recommendation:   recommendation,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := p.repo.Create(ctx, assessment); err != nil {
		return nil, fmt.Errorf("persist assessment: %w", err)
	}
	return assessment, nil
}

// runRules merges declarative-rule output into the domain RuleTriggered list.
// If no engine is configured, it returns an empty slice.
func (p *Pipeline) runRules(features domain.Features, extra map[string]any) []domain.RuleTriggered {
	if p.rulesEngine == nil {
		return []domain.RuleTriggered{}
	}
	facts := features.AsFacts()
	for k, v := range extra { // caller-provided facts win — they are more specific
		facts[k] = v
	}
	res := p.rulesEngine.Evaluate(facts)
	out := make([]domain.RuleTriggered, 0, len(res.FiredRules))
	blockedSet := make(map[string]struct{}, len(res.Flags))
	if res.Blocked {
		// Block action also adds to Flags; collect them here so we can mark
		// the corresponding RuleTriggered entries BLOCKING.
		for _, f := range res.Flags {
			blockedSet[f] = struct{}{}
		}
	}
	for _, ruleID := range res.FiredRules {
		severity := domain.SeverityWarning
		if _, isBlocking := blockedSet[ruleID]; isBlocking && res.Blocked {
			severity = domain.SeverityBlocking
		}
		out = append(out, domain.RuleTriggered{
			RuleID:      ruleID,
			Severity:    severity,
			Description: humanizeRuleID(ruleID),
		})
	}
	return out
}

// DecideRecommendation is exported so handlers/tests can reuse it without
// pulling in the full pipeline. Logic from docs/technical-structure.md § 4.2.
func DecideRecommendation(score float64, rules []domain.RuleTriggered) domain.Recommendation {
	hasBlocking := false
	hasWarning := false
	for _, r := range rules {
		switch r.Severity {
		case domain.SeverityBlocking:
			hasBlocking = true
		case domain.SeverityWarning:
			hasWarning = true
		}
	}
	if hasBlocking || score > 0.7 {
		return domain.RecommendDeclineRecommended
	}
	if score < 0.3 && !hasWarning {
		return domain.RecommendAutoApprove
	}
	return domain.RecommendManualReview
}

// emptyScreening returns a placeholder screening_results value for the MVP.
// In production both blocks are populated by ext-rosfinmon / ext-fssp before
// the pipeline runs.
func emptyScreening(now time.Time) domain.ScreeningResults {
	return domain.ScreeningResults{
		Rosfinmon: domain.ScreeningResult{
			Source: "ROSFINMON", CheckedAt: now, HasMatches: false, Matches: []domain.ScreeningMatch{},
		},
		FSSP: domain.ScreeningResult{
			Source: "FSSP", CheckedAt: now, HasMatches: false, Matches: []domain.ScreeningMatch{},
		},
	}
}

func humanizeRuleID(id string) string {
	// Lightweight transform: BLOCK_HIGH_RISK_OKVED → "Block high risk okved".
	// Real descriptions come from the rule definition once we wire rule names
	// through ruleengine.EvalResult — tracked in TODO inside packages/rule-engine.
	if id == "" {
		return ""
	}
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(id, "_", " ")), " ")
	if len(parts) == 0 {
		return id
	}
	parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return strings.Join(parts, " ")
}

package domain

import "time"

// RiskCategory mirrors domain-model.md § 2.7.
type RiskCategory string

const (
	CategoryLow    RiskCategory = "LOW"
	CategoryMedium RiskCategory = "MEDIUM"
	CategoryHigh   RiskCategory = "HIGH"
)

// Direction explains whether a SHAP-style factor pushes the score up or down.
type Direction string

const (
	IncreasesRisk Direction = "INCREASES_RISK"
	DecreasesRisk Direction = "DECREASES_RISK"
)

// RuleSeverity copies the severity ladder from rule-engine but keeps it as a
// domain-level concern (rules_triggered in domain-model.md).
type RuleSeverity string

const (
	SeverityWarning  RuleSeverity = "WARNING"
	SeverityBlocking RuleSeverity = "BLOCKING"
)

// Recommendation enum — what the engine suggests downstream services do.
type Recommendation string

const (
	RecommendAutoApprove        Recommendation = "AUTO_APPROVE"
	RecommendManualReview       Recommendation = "MANUAL_REVIEW"
	RecommendDeclineRecommended Recommendation = "DECLINE_RECOMMENDED"
)

// Factor is a single SHAP-like contribution.
type Factor struct {
	Name      string    `json:"name"`
	Value     any       `json:"value"`
	Weight    float64   `json:"weight"`
	Direction Direction `json:"direction"`
}

// RuleTriggered is a hard rule outcome from the rule-engine.
type RuleTriggered struct {
	RuleID      string       `json:"rule_id"`
	Severity    RuleSeverity `json:"severity"`
	Description string       `json:"description"`
}

// ScreeningMatch — single hit on an external screening list.
type ScreeningMatch struct {
	ListName string  `json:"list_name"`
	Field    string  `json:"matched_field"`
	Score    float64 `json:"confidence"`
}

// ScreeningResult is a generic source-keyed screening verdict.
type ScreeningResult struct {
	Source     string           `json:"source"`
	CheckedAt  time.Time        `json:"checked_at"`
	HasMatches bool             `json:"has_matches"`
	Matches    []ScreeningMatch `json:"matches"`
}

// ScreeningResults groups per-source results required by domain-model.md § 2.7.
type ScreeningResults struct {
	Rosfinmon ScreeningResult `json:"rosfinmon"`
	FSSP      ScreeningResult `json:"fssp"`
}

// ModelInfo describes which model produced the score.
type ModelInfo struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	ComputedAt time.Time `json:"computed_at"`
}

// Explanation contains the human-readable text for the compliance officer.
type Explanation struct {
	Text        string `json:"text"`
	GeneratedBy string `json:"generated_by"`
}

// RiskAssessment is the persisted result of one scoring run.
// Mirrors domain-model.md § 2.7 (1:1 fields).
type RiskAssessment struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	LegalEntityID string `json:"legal_entity_id,omitempty"`

	Score    float64      `json:"score"` // 0..1
	Category RiskCategory `json:"category"`

	Model            ModelInfo        `json:"model"`
	Factors          []Factor         `json:"factors"`
	RulesTriggered   []RuleTriggered  `json:"rules_triggered"`
	ScreeningResults ScreeningResults `json:"screening_results"`
	Explanation      Explanation      `json:"explanation"`
	Recommendation   Recommendation   `json:"recommendation"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CategoryFromScore is the canonical bucketing used by both scorer and
// pipeline so they can never disagree.
func CategoryFromScore(score float64) RiskCategory {
	switch {
	case score < 0.3:
		return CategoryLow
	case score < 0.7:
		return CategoryMedium
	default:
		return CategoryHigh
	}
}

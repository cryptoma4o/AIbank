package ruleengine_test

import (
	"testing"

	ruleengine "aibank/rule-engine"
)

const testRulesYAML = `
version: "1.0"
rules:
  - id: BLOCK_HIGH_RISK_OKVED
    name: Block high-risk OKVED codes
    enabled: true
    priority: 1
    conditions:
      - field: okved
        operator: in
        value: ["64.19", "66.19", "47.91"]
    action: block
    weight: 0
  - id: FLAG_LOW_SCORE
    name: Flag applications with low risk score
    enabled: true
    priority: 10
    conditions:
      - field: risk_score
        operator: lt
        value: 50
    action: flag
    weight: 0
  - id: SCORE_BOOST_ESTABLISHED
    name: Boost score for established companies (>5 years)
    enabled: true
    priority: 20
    conditions:
      - field: company_age_years
        operator: gt
        value: 5
    action: score
    weight: 10
`

func TestBlock(t *testing.T) {
	eng, err := ruleengine.NewEngineFromYAML([]byte(testRulesYAML))
	if err != nil {
		t.Fatalf("NewEngineFromYAML: %v", err)
	}
	result := eng.Evaluate(map[string]any{
		"okved":             "64.19",
		"risk_score":        70,
		"company_age_years": 3,
	})
	if !result.Blocked {
		t.Error("expected blocked=true for high-risk OKVED")
	}
}

func TestFlag(t *testing.T) {
	eng, _ := ruleengine.NewEngineFromYAML([]byte(testRulesYAML))
	result := eng.Evaluate(map[string]any{
		"okved":             "62.01",
		"risk_score":        30,
		"company_age_years": 2,
	})
	if result.Blocked {
		t.Error("expected not blocked")
	}
	if len(result.Flags) == 0 {
		t.Error("expected at least one flag")
	}
}

func TestScoreAdjust(t *testing.T) {
	eng, _ := ruleengine.NewEngineFromYAML([]byte(testRulesYAML))
	result := eng.Evaluate(map[string]any{
		"okved":             "62.01",
		"risk_score":        80,
		"company_age_years": 10,
	})
	if result.ScoreAdjust != 10 {
		t.Errorf("expected ScoreAdjust=10, got %d", result.ScoreAdjust)
	}
}

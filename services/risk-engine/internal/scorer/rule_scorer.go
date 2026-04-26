package scorer

import (
	"fmt"
	"os"

	ruleengine "aibank/risk-engine/internal/ruleengine"
)

type RuleScorer struct {
	engine *ruleengine.Engine
}

func NewRuleScorerFromFile(path string) (*RuleScorer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("rule scorer: %w", err)
	}
	eng, err := ruleengine.NewEngineFromYAML(data)
	if err != nil {
		return nil, fmt.Errorf("rule scorer: %w", err)
	}
	return &RuleScorer{engine: eng}, nil
}

func (r *RuleScorer) Evaluate(facts map[string]any) ruleengine.EvalResult {
	return r.engine.Evaluate(facts)
}

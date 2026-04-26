package ruleengine

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type EvalResult struct {
	Blocked      bool
	Flags        []string
	RequiredDocs []string
	ScoreAdjust  int
	FiredRules   []string
}

type Engine struct {
	rules []Rule
}

func NewEngineFromFile(path string) (*Engine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("rule-engine: read file: %w", err)
	}
	return NewEngineFromYAML(data)
}

func NewEngineFromYAML(data []byte) (*Engine, error) {
	var rs RuleSet
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("rule-engine: parse yaml: %w", err)
	}
	rules := make([]Rule, 0, len(rs.Rules))
	for _, r := range rs.Rules {
		if r.Enabled {
			rules = append(rules, r)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})
	return &Engine{rules: rules}, nil
}

// Evaluate runs all enabled rules against the provided facts map.
// facts keys are dot-notation field paths, e.g. "applicant.inn", "score", "okved".
func (e *Engine) Evaluate(facts map[string]any) EvalResult {
	result := EvalResult{}
	for _, rule := range e.rules {
		if e.matchAll(rule.Conditions, facts) {
			result.FiredRules = append(result.FiredRules, rule.ID)
			switch rule.Action {
			case ActionBlock:
				result.Blocked = true
				result.Flags = append(result.Flags, rule.ID)
			case ActionFlag:
				result.Flags = append(result.Flags, rule.ID)
			case ActionRequiresDocs:
				result.RequiredDocs = append(result.RequiredDocs, rule.ID)
			case ActionScore:
				result.ScoreAdjust += rule.Weight
			}
		}
	}
	return result
}

func (e *Engine) matchAll(conditions []Condition, facts map[string]any) bool {
	for _, c := range conditions {
		if !e.matchOne(c, facts) {
			return false
		}
	}
	return true
}

func (e *Engine) matchOne(c Condition, facts map[string]any) bool {
	val, ok := facts[c.Field]
	if !ok {
		return false
	}
	switch c.Operator {
	case OpEquals:
		return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", c.Value)
	case OpNotEquals:
		return fmt.Sprintf("%v", val) != fmt.Sprintf("%v", c.Value)
	case OpGreaterThan:
		return toFloat(val) > toFloat(c.Value)
	case OpLessThan:
		return toFloat(val) < toFloat(c.Value)
	case OpIn:
		list := toStringSlice(c.Value)
		return slices.Contains(list, fmt.Sprintf("%v", val))
	case OpNotIn:
		list := toStringSlice(c.Value)
		return !slices.Contains(list, fmt.Sprintf("%v", val))
	case OpContains:
		return strings.Contains(fmt.Sprintf("%v", val), fmt.Sprintf("%v", c.Value))
	}
	return false
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, len(t))
		for i, item := range t {
			out[i] = fmt.Sprintf("%v", item)
		}
		return out
	case []string:
		return t
	}
	return nil
}

package ruleengine

// Operator for condition comparison
type Operator string

const (
	OpEquals      Operator = "eq"
	OpNotEquals   Operator = "neq"
	OpGreaterThan Operator = "gt"
	OpLessThan    Operator = "lt"
	OpIn          Operator = "in"
	OpNotIn       Operator = "not_in"
	OpContains    Operator = "contains"
)

// Action when rule fires
type Action string

const (
	ActionBlock        Action = "block"
	ActionFlag         Action = "flag"
	ActionRequiresDocs Action = "requires_docs"
	ActionScore        Action = "score" // adjusts score by Weight
)

type Condition struct {
	Field    string   `yaml:"field"`
	Operator Operator `yaml:"operator"`
	Value    any      `yaml:"value"`
}

type Rule struct {
	ID          string      `yaml:"id"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Enabled     bool        `yaml:"enabled"`
	Priority    int         `yaml:"priority"`   // lower = evaluated first
	Conditions  []Condition `yaml:"conditions"` // AND logic
	Action      Action      `yaml:"action"`
	Weight      int         `yaml:"weight"` // for action: score
}

type RuleSet struct {
	Version string `yaml:"version"`
	Rules   []Rule `yaml:"rules"`
}

package scorer

import (
	"strings"
	"testing"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

func TestTextExplainer_ContainsCategoryAndScore(t *testing.T) {
	t.Parallel()

	e := NewTextExplainer()
	exp := e.Explain(0.42, nil, nil)
	if !strings.Contains(exp.Text, "0.42") {
		t.Errorf("explanation missing score: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "MEDIUM") {
		t.Errorf("explanation missing category: %q", exp.Text)
	}
	if exp.GeneratedBy == "" {
		t.Errorf("generated_by must be set")
	}
}

func TestTextExplainer_PositiveAndNegativeFactors(t *testing.T) {
	t.Parallel()

	e := NewTextExplainer()
	factors := []domain.Factor{
		{Name: "company_age_years", Value: 8.0, Weight: -0.18, Direction: domain.DecreasesRisk},
		{Name: "has_blocked_okved", Value: true, Weight: 0.40, Direction: domain.IncreasesRisk},
		{Name: "noise", Value: 0, Weight: 0, Direction: domain.IncreasesRisk}, // dropped
	}
	exp := e.Explain(0.55, factors, nil)
	if !strings.Contains(exp.Text, "Положительные факторы") {
		t.Errorf("missing positive section: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "Отрицательные факторы") {
		t.Errorf("missing negative section: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "company_age_years") {
		t.Errorf("missing positive factor name: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "has_blocked_okved") {
		t.Errorf("missing negative factor name: %q", exp.Text)
	}
	if strings.Contains(exp.Text, "noise") {
		t.Errorf("zero-weight factor leaked into explanation: %q", exp.Text)
	}
}

func TestTextExplainer_RulesSectionAppears(t *testing.T) {
	t.Parallel()

	e := NewTextExplainer()
	rules := []domain.RuleTriggered{
		{RuleID: "BLOCK_HIGH_RISK_OKVED", Severity: domain.SeverityBlocking, Description: "ОКВЭД заблокирован"},
	}
	exp := e.Explain(0.85, nil, rules)
	if !strings.Contains(exp.Text, "Сработавшие правила") {
		t.Errorf("missing rules section: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "ОКВЭД заблокирован") {
		t.Errorf("missing rule description: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "BLOCKING") {
		t.Errorf("missing severity: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "HIGH") {
		t.Errorf("missing high category: %q", exp.Text)
	}
}

func TestTextExplainer_NoFactorsNoRules(t *testing.T) {
	t.Parallel()

	e := NewTextExplainer()
	exp := e.Explain(0.10, nil, nil)
	// Should still contain the prefix line.
	if !strings.HasPrefix(exp.Text, "Скор") {
		t.Errorf("explanation must start with score line: %q", exp.Text)
	}
	if !strings.Contains(exp.Text, "LOW") {
		t.Errorf("missing low category: %q", exp.Text)
	}
}

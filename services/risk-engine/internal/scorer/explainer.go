package scorer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

// Explainer turns the scorer + rule-engine outputs into a Russian-language
// explanation suitable for the compliance officer UI (см. domain-model.md
// § 2.7 — поле explanation).
type Explainer interface {
	Explain(score float64, factors []domain.Factor, rulesTriggered []domain.RuleTriggered) domain.Explanation
}

// TextExplainer is a pure-Go template renderer. No LLM calls — predictable,
// auditable, regulator-friendly. The model field still records who generated
// the text so future LLM-backed explainers slot in cleanly.
type TextExplainer struct {
	GeneratedBy string
}

// NewTextExplainer returns the default plain-text explainer.
func NewTextExplainer() *TextExplainer {
	return &TextExplainer{GeneratedBy: "rule-based-text-explainer-v1"}
}

// Explain composes the explanation. The first line always contains the score
// and category, so log-line scrapers and downstream UIs can rely on it.
func (e *TextExplainer) Explain(score float64, factors []domain.Factor, rules []domain.RuleTriggered) domain.Explanation {
	category := domain.CategoryFromScore(score)

	var b strings.Builder
	fmt.Fprintf(&b, "Скор %.2f — категория %s.", score, categoryRu(category))

	pos, neg := splitFactors(factors)
	if len(pos) > 0 {
		fmt.Fprintf(&b, " Положительные факторы: %s.", joinFactors(pos))
	}
	if len(neg) > 0 {
		fmt.Fprintf(&b, " Отрицательные факторы: %s.", joinFactors(neg))
	}
	if len(rules) > 0 {
		fmt.Fprintf(&b, " Сработавшие правила: %s.", joinRules(rules))
	}

	return domain.Explanation{Text: b.String(), GeneratedBy: e.GeneratedBy}
}

// splitFactors returns (decreasing_risk, increasing_risk), with non-zero
// weights only — zero contributions add noise to the explanation.
func splitFactors(factors []domain.Factor) (pos, neg []domain.Factor) {
	for _, f := range factors {
		if f.Weight == 0 {
			continue
		}
		switch f.Direction {
		case domain.DecreasesRisk:
			pos = append(pos, f)
		case domain.IncreasesRisk:
			neg = append(neg, f)
		}
	}
	// Sort by absolute weight, strongest first — humans skim from the top.
	sortByAbsWeight := func(s []domain.Factor) {
		sort.SliceStable(s, func(i, j int) bool {
			return abs(s[i].Weight) > abs(s[j].Weight)
		})
	}
	sortByAbsWeight(pos)
	sortByAbsWeight(neg)
	return pos, neg
}

func joinFactors(fs []domain.Factor) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, fmt.Sprintf("%s (вес %.2f)", f.Name, f.Weight))
	}
	return strings.Join(parts, ", ")
}

func joinRules(rs []domain.RuleTriggered) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		desc := r.Description
		if desc == "" {
			desc = r.RuleID
		}
		parts = append(parts, fmt.Sprintf("%s [%s]", desc, r.Severity))
	}
	return strings.Join(parts, "; ")
}

func categoryRu(c domain.RiskCategory) string {
	// We keep the English enum value alongside the Russian gloss because that
	// is the format compliance UIs expect — see docs/domain-model.md § 2.7.
	switch c {
	case domain.CategoryLow:
		return "LOW (низкий)"
	case domain.CategoryMedium:
		return "MEDIUM (средний)"
	case domain.CategoryHigh:
		return "HIGH (высокий)"
	default:
		return string(c)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

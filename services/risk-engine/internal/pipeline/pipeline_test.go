package pipeline_test

import (
	"context"
	"sync"
	"testing"
	"time"

	ruleengine "aibank/rule-engine"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
	"github.com/aibank/platform/services/risk-engine/internal/pipeline"
	"github.com/aibank/platform/services/risk-engine/internal/scorer"
)

// inMemoryRepo is a thread-safe in-memory RiskAssessmentRepository.
type inMemoryRepo struct {
	mu    sync.Mutex
	items []*domain.RiskAssessment
}

func (r *inMemoryRepo) Create(_ context.Context, a *domain.RiskAssessment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, a)
	return nil
}

func (r *inMemoryRepo) GetByID(_ context.Context, tenantID, id string) (*domain.RiskAssessment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, it := range r.items {
		if it.ID == id && it.TenantID == tenantID {
			return it, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *inMemoryRepo) ListByApplication(_ context.Context, tenantID, applicationID string) ([]*domain.RiskAssessment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.RiskAssessment, 0)
	for _, it := range r.items {
		if it.TenantID == tenantID && it.ApplicationID == applicationID {
			out = append(out, it)
		}
	}
	return out, nil
}

// fixedScorer always returns the configured score and a fixed factor list.
type fixedScorer struct {
	score   float64
	factors []domain.Factor
	model   domain.ModelInfo
}

func (f fixedScorer) Score(_ domain.Features) (float64, []domain.Factor, error) {
	return f.score, f.factors, nil
}
func (f fixedScorer) ModelInfo() domain.ModelInfo { return f.model }

func newPipeline(t *testing.T, score float64, rulesYAML string) (*pipeline.Pipeline, *inMemoryRepo) {
	t.Helper()
	repo := &inMemoryRepo{}
	var eng *ruleengine.Engine
	if rulesYAML != "" {
		var err error
		eng, err = ruleengine.NewEngineFromYAML([]byte(rulesYAML))
		if err != nil {
			t.Fatalf("rules parse: %v", err)
		}
	}
	p := pipeline.NewPipeline(pipeline.Config{
		Extractor: domain.NewMapExtractor(),
		Scorer: fixedScorer{
			score: score,
			factors: []domain.Factor{
				{Name: "company_age_years", Weight: -0.1, Direction: domain.DecreasesRisk},
			},
			model: domain.ModelInfo{Name: "fixed", Version: "test"},
		},
		Explainer: scorer.NewTextExplainer(),
		Repo:      repo,
		Rules:     eng,
		Clock:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		IDGen:     func() string { return "ra_test" },
	})
	return p, repo
}

func TestPipeline_HappyPath_AutoApprove(t *testing.T) {
	t.Parallel()

	p, repo := newPipeline(t, 0.10, "")
	a, err := p.Assess(context.Background(), "alfa", "app_1", map[string]any{
		"company_age_years": 8.0,
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if a.Recommendation != domain.RecommendAutoApprove {
		t.Errorf("expected AUTO_APPROVE, got %s", a.Recommendation)
	}
	if a.Category != domain.CategoryLow {
		t.Errorf("expected LOW category, got %s", a.Category)
	}
	if len(repo.items) != 1 {
		t.Errorf("expected one persisted item, got %d", len(repo.items))
	}
}

func TestPipeline_HighScore_Decline(t *testing.T) {
	t.Parallel()

	p, _ := newPipeline(t, 0.85, "")
	a, err := p.Assess(context.Background(), "alfa", "app_2", nil)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if a.Recommendation != domain.RecommendDeclineRecommended {
		t.Errorf("expected DECLINE_RECOMMENDED, got %s", a.Recommendation)
	}
	if a.Category != domain.CategoryHigh {
		t.Errorf("expected HIGH category, got %s", a.Category)
	}
}

func TestPipeline_BlockingRuleOverridesLowScore(t *testing.T) {
	t.Parallel()

	rulesYAML := `
version: "1.0"
rules:
  - id: BLOCK_ROSFINMON
    name: Block on Rosfinmon list
    enabled: true
    priority: 1
    conditions:
      - field: rosfinmon_hit
        operator: eq
        value: true
    action: block
    weight: 0
`
	p, _ := newPipeline(t, 0.10, rulesYAML) // numerically AUTO_APPROVE
	a, err := p.Assess(context.Background(), "alfa", "app_3", map[string]any{
		"rosfinmon_hit": true,
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if a.Recommendation != domain.RecommendDeclineRecommended {
		t.Errorf("blocking rule must override low score, got %s", a.Recommendation)
	}
	if len(a.RulesTriggered) != 1 {
		t.Fatalf("expected one triggered rule, got %d", len(a.RulesTriggered))
	}
	if a.RulesTriggered[0].Severity != domain.SeverityBlocking {
		t.Errorf("expected blocking severity, got %s", a.RulesTriggered[0].Severity)
	}
}

func TestPipeline_MediumScore_ManualReview(t *testing.T) {
	t.Parallel()

	p, _ := newPipeline(t, 0.5, "")
	a, err := p.Assess(context.Background(), "alfa", "app_4", nil)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if a.Recommendation != domain.RecommendManualReview {
		t.Errorf("expected MANUAL_REVIEW, got %s", a.Recommendation)
	}
}

func TestPipeline_RequiresTenantAndApp(t *testing.T) {
	t.Parallel()

	p, _ := newPipeline(t, 0.1, "")
	if _, err := p.Assess(context.Background(), "", "app", nil); err == nil {
		t.Errorf("expected error for empty tenant")
	}
	if _, err := p.Assess(context.Background(), "alfa", "", nil); err == nil {
		t.Errorf("expected error for empty application")
	}
}

func TestDecideRecommendation_Table(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		score    float64
		rules    []domain.RuleTriggered
		expected domain.Recommendation
	}{
		{"low no rules", 0.1, nil, domain.RecommendAutoApprove},
		{"low warning rule", 0.1,
			[]domain.RuleTriggered{{RuleID: "X", Severity: domain.SeverityWarning}},
			domain.RecommendManualReview},
		{"medium no rules", 0.5, nil, domain.RecommendManualReview},
		{"high no rules", 0.85, nil, domain.RecommendDeclineRecommended},
		{"low blocking rule", 0.1,
			[]domain.RuleTriggered{{RuleID: "B", Severity: domain.SeverityBlocking}},
			domain.RecommendDeclineRecommended},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pipeline.DecideRecommendation(tc.score, tc.rules)
			if got != tc.expected {
				t.Errorf("score=%v rules=%v: got %s, want %s", tc.score, tc.rules, got, tc.expected)
			}
		})
	}
}

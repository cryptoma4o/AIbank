package scorer

import (
	"testing"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

func TestStubScorer_DeterministicSameInputs(t *testing.T) {
	t.Parallel()

	s := NewStubScorer()
	f := domain.Features{
		CompanyAgeYears: 7,
		HasBlockedOKVED: false,
		UBOCount:        2,
		CapitalKopecks:  1_000_000_00,
		OKVED:           "62.01",
	}

	score1, factors1, err := s.Score(f)
	if err != nil {
		t.Fatalf("score1: %v", err)
	}
	score2, factors2, err := s.Score(f)
	if err != nil {
		t.Fatalf("score2: %v", err)
	}
	if score1 != score2 {
		t.Errorf("non-deterministic: %v vs %v", score1, score2)
	}
	if len(factors1) != len(factors2) {
		t.Fatalf("factors length differs: %d vs %d", len(factors1), len(factors2))
	}
	for i := range factors1 {
		if factors1[i].Weight != factors2[i].Weight {
			t.Errorf("factor %d weight diverged: %v vs %v", i, factors1[i], factors2[i])
		}
	}
}

func TestStubScorer_DifferentInputsDifferentScores(t *testing.T) {
	t.Parallel()

	s := NewStubScorer()
	low := domain.Features{CompanyAgeYears: 10, UBOCount: 2}
	high := domain.Features{CompanyAgeYears: 0, UBOCount: 6, HasBlockedOKVED: true, HasForeignOwners: true}

	lowScore, _, _ := s.Score(low)
	highScore, _, _ := s.Score(high)
	if lowScore == highScore {
		t.Errorf("expected different scores for different inputs, got %v == %v", lowScore, highScore)
	}
}

func TestStubScorer_ScoreRange(t *testing.T) {
	t.Parallel()

	s := NewStubScorer()
	cases := []domain.Features{
		{},                                      // all zero
		{CompanyAgeYears: 50, UBOCount: 3},      // very mature
		{HasBlockedOKVED: true, UBOCount: 0},    // pathological
		{CompanyAgeYears: 1, UBOCount: 100},     // odd outlier
		{CapitalKopecks: 100_000_000, UBOCount: 4},
	}
	for i, f := range cases {
		score, _, err := s.Score(f)
		if err != nil {
			t.Fatalf("case %d: error %v", i, err)
		}
		if score < 0 || score > 1 {
			t.Errorf("case %d: score out of range: %v", i, score)
		}
	}
}

func TestStubScorer_FactorsNonEmptyAndShaped(t *testing.T) {
	t.Parallel()

	s := NewStubScorer()
	score, factors, err := s.Score(domain.Features{
		CompanyAgeYears: 3,
		HasBlockedOKVED: true,
		UBOCount:        2,
	})
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	if score < 0 || score > 1 {
		t.Fatalf("score out of range: %v", score)
	}
	if len(factors) == 0 {
		t.Fatalf("expected at least one factor, got 0")
	}
	wantNames := map[string]bool{
		"company_age_years": false, "has_blocked_okved": false,
		"registration_address_is_mass": false, "ubo_count": false,
		"has_foreign_owners": false,
	}
	for _, fct := range factors {
		if _, ok := wantNames[fct.Name]; ok {
			wantNames[fct.Name] = true
		}
		if fct.Direction != domain.IncreasesRisk && fct.Direction != domain.DecreasesRisk {
			t.Errorf("factor %s: invalid direction %q", fct.Name, fct.Direction)
		}
	}
	for name, present := range wantNames {
		if !present {
			t.Errorf("expected factor %s, missing", name)
		}
	}
}

func TestStubScorer_ModelInfo(t *testing.T) {
	t.Parallel()

	s := NewStubScorer()
	mi := s.ModelInfo()
	if mi.Name == "" || mi.Version == "" {
		t.Errorf("model info should be filled, got %+v", mi)
	}
}

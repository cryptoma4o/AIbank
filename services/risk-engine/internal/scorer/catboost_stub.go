package scorer

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

// CatBoostScorer is the abstraction the pipeline depends on.
// docs/technical-structure.md § 7.2 fixes CatBoost as the production model
// for risk scoring (NOT an LLM) — this interface keeps the door open for the
// real .cbm runtime to drop in without touching pipeline code.
type CatBoostScorer interface {
	Score(features domain.Features) (score float64, factors []domain.Factor, err error)
	ModelInfo() domain.ModelInfo
}

// StubScorer is the MVP implementation used until a real CatBoost runtime is
// wired in. Properties:
//
//   - Deterministic: the same Features always produce the same score.
//   - Score in [0, 1].
//   - Returns 5 plausible SHAP-style factors derived from the inputs.
//
// TODO(v2 — real CatBoost): replace this struct with one that
//   1. Loads a `.cbm` file from disk (path injected via env / tenant config).
//   2. Calls into ONNX runtime or the official catboost-go cgo bindings.
//   3. Computes real SHAP values via `model.GetFeatureImportance(SHAPValues)`.
//   4. Caches per-feature contribution metadata for explainer.
//
// The interface above is the seam — nothing else has to change.
type StubScorer struct {
	model domain.ModelInfo
}

// NewStubScorer returns the MVP CatBoost stand-in.
func NewStubScorer() *StubScorer {
	return &StubScorer{
		model: domain.ModelInfo{
			Name:    "catboost-onboarding-stub",
			Version: "0.1.0",
		},
	}
}

// ModelInfo returns the metadata persisted alongside the assessment.
// computed_at is filled in by the pipeline.
func (s *StubScorer) ModelInfo() domain.ModelInfo { return s.model }

// Score derives a deterministic risk score from the feature vector.
//
// We blend two signals:
//  1. A hash bucket — guarantees determinism across runs and a uniform spread
//     when features are diverse (used in tests and as a tiebreaker).
//  2. A weighted sum of well-known risk drivers — gives the score a sensible
//     monotonic relationship with the inputs (older companies ⇒ less risky,
//     blocked OKVED ⇒ more risky), so explainer text doesn't lie.
func (s *StubScorer) Score(features domain.Features) (float64, []domain.Factor, error) {
	hashBucket := stableHash01(features)

	// Driver contributions — see TODO above; weights are illustrative until a
	// real CatBoost model lands.
	type driver struct {
		name      string
		value     any
		weight    float64
		direction domain.Direction
	}
	drivers := []driver{
		{
			name:      "company_age_years",
			value:     features.CompanyAgeYears,
			weight:    -0.20 * clamp(features.CompanyAgeYears/10.0, 0, 1),
			direction: domain.DecreasesRisk,
		},
		{
			name:      "has_blocked_okved",
			value:     features.HasBlockedOKVED,
			weight:    boolWeight(features.HasBlockedOKVED, +0.40),
			direction: domain.IncreasesRisk,
		},
		{
			name:      "registration_address_is_mass",
			value:     features.RegistrationAddressIsMass,
			weight:    boolWeight(features.RegistrationAddressIsMass, +0.15),
			direction: domain.IncreasesRisk,
		},
		{
			name:      "ubo_count",
			value:     features.UBOCount,
			weight:    uboWeight(features.UBOCount),
			direction: uboDirection(features.UBOCount),
		},
		{
			name:      "has_foreign_owners",
			value:     features.HasForeignOwners,
			weight:    boolWeight(features.HasForeignOwners, +0.10),
			direction: domain.IncreasesRisk,
		},
	}

	driverSum := 0.0
	factors := make([]domain.Factor, 0, len(drivers))
	for _, d := range drivers {
		driverSum += d.weight
		factors = append(factors, domain.Factor{
			Name:      d.name,
			Value:     d.value,
			Weight:    round4(d.weight),
			Direction: d.direction,
		})
	}

	// Final score: 50% hash bucket, 50% drivers shifted into [0, 1]; this keeps
	// the function deterministic AND responsive to inputs.
	score := 0.5*hashBucket + 0.5*clamp(0.5+driverSum, 0, 1)
	score = clamp(score, 0, 1)
	return round4(score), factors, nil
}

// stableHash01 maps the entire feature vector to a deterministic value in [0, 1].
func stableHash01(f domain.Features) float64 {
	h := sha256.New()
	// Compose a stable representation. fmt.Fprintf is enough — we don't need to
	// guard against locale-dependent floats since all features are domain ints,
	// bools, or whole-number floats coming from upstream JSON.
	fmt.Fprintf(h, "%v|%v|%v|%v|%v|%v|%v|%v|%v|%v",
		f.CompanyAgeYears, f.HasBlockedOKVED, f.RegistrationAddressIsMass,
		f.UBOCount, f.HasForeignOwners, f.CapitalKopecks,
		f.HasRosfinmonHit, f.HasFSSPHit, f.IsInBankruptcy, f.OKVED,
	)
	sum := h.Sum(nil)
	// First 8 bytes give us a uint64 with plenty of entropy for [0, 1] mapping.
	u := binary.BigEndian.Uint64(sum[:8])
	return float64(u) / float64(^uint64(0))
}

func boolWeight(b bool, w float64) float64 {
	if b {
		return w
	}
	return 0
}

func uboWeight(count int) float64 {
	switch {
	case count == 0:
		return +0.20 // no UBOs disclosed → suspicious
	case count >= 5:
		return +0.10 // many UBOs → complex ownership
	default:
		return -0.05 // 1..4 UBOs → typical, slightly positive signal
	}
}

func uboDirection(count int) domain.Direction {
	if count == 0 || count >= 5 {
		return domain.IncreasesRisk
	}
	return domain.DecreasesRisk
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round4(v float64) float64 {
	// Keep the contract numerically stable across platforms; 4 decimals is
	// enough precision for an MVP scorer and gives nicer JSON.
	return float64(int64(v*10000+sign(v)*0.5)) / 10000
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

package scorer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

// ErrONNXNotWired returned when ONNXScorer.Score is called before the real
// onnxruntime-go integration is in place. Pipeline can detect this and either
// fall back to StubScorer or surface as 503 to the caller.
var ErrONNXNotWired = errors.New(
	"ONNX inference not wired — see TODO in scorer/onnx_scorer.go for steps")

// ONNXScorer is the seam for the real CatBoost-via-ONNX inference path.
//
// Status: SKELETON. The constructor validates the model file exists and reads
// metadata (size, SHA-256 checksum used as a stable version identifier), so
// the rest of the platform can already select between stub and onnx via env.
//
// Score() returns ErrONNXNotWired with a non-empty Factor slice (zero
// weights, neutral direction) so explainer/pipeline can surface the model
// metadata without crashing. The real implementation lands when the
// onnxruntime-go runtime + a trained CatBoost model file are available.
//
// TODO(v2 real ONNX integration):
//
//  1. Add `github.com/yalue/onnxruntime_go` v1.10+ to go.mod.
//  2. Initialise `onnxruntime_go.InitializeEnvironment()` at process start
//     (idempotent, sync.Once) and ensure DestroyEnvironment on shutdown.
//  3. In NewONNXScorer: read the model file via `onnxruntime_go.NewSession`,
//     bind to a single named input tensor "features" (FLOAT32, shape [1, N])
//     and named output tensors "score" (FLOAT32 [1]) and "shap_values"
//     (FLOAT32 [1, N+1] — last column is bias). CatBoost's ONNX exporter
//     produces this layout when invoked with `--output-type=ONNX
//     --output-tree-stats`.
//  4. In Score: convert Features into a deterministic []float32 vector
//     using featuresToVector (already exists below — keep same field order
//     so model retraining picks up the same schema). Call sess.Run.
//  5. Build []domain.Factor from the SHAP output: feature_name + signed
//     weight + direction (INCREASES_RISK if weight > 0).
//  6. Cache the loaded session — onnxruntime sessions are thread-safe and
//     should live for the process lifetime.
//
// References:
//   - docs/technical-structure.md § 7.2 (CatBoost для risk-scoring)
//   - docs/adr/0011-llm-routing-strategy.md (model lifecycle, eval gates)
//   - configs/risk-models/README.md (where production models live)
type ONNXScorer struct {
	modelPath string
	model     domain.ModelInfo
}

// NewONNXScorer prepares the scorer.
// Validates that:
//   - the file exists and is non-empty
//   - extension is .onnx
//
// Returns the scorer with metadata populated; Score() returns ErrONNXNotWired
// until the real runtime is wired (see TODO above).
func NewONNXScorer(modelPath string) (*ONNXScorer, error) {
	if modelPath == "" {
		return nil, errors.New("ONNXScorer: model_path is required")
	}
	ext := strings.ToLower(filepath.Ext(modelPath))
	if ext != ".onnx" {
		return nil, fmt.Errorf("ONNXScorer: unsupported extension %q (expected .onnx)", ext)
	}
	st, err := os.Stat(modelPath)
	if err != nil {
		return nil, fmt.Errorf("ONNXScorer: stat %s: %w", modelPath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("ONNXScorer: %s is a directory, not a file", modelPath)
	}
	if st.Size() == 0 {
		return nil, fmt.Errorf("ONNXScorer: model file %s is empty", modelPath)
	}

	// SHA-256 of the model bytes is a stable, content-addressed version.
	// Useful for audit log: every Score event records this version, so we
	// know exactly which weights produced which decision.
	f, err := os.Open(modelPath)
	if err != nil {
		return nil, fmt.Errorf("ONNXScorer: open %s: %w", modelPath, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("ONNXScorer: hash %s: %w", modelPath, err)
	}
	checksum := hex.EncodeToString(h.Sum(nil))[:12]

	// Filename convention from configs/risk-models/README.md: risk-vMAJOR.MINOR.PATCH.onnx
	// We use the basename without extension if it matches "risk-v*", else fall back to checksum.
	baseName := strings.TrimSuffix(filepath.Base(modelPath), ext)
	version := baseName
	if !strings.HasPrefix(baseName, "risk-v") {
		version = "sha256:" + checksum
	}

	return &ONNXScorer{
		modelPath: modelPath,
		model: domain.ModelInfo{
			Name:       "catboost-onboarding",
			Version:    version,
			ComputedAt: time.Now().UTC(),
		},
	}, nil
}

// Score returns ErrONNXNotWired with a metadata-only Factor slice (zero weights).
// This lets the pipeline / explainer code path stay structurally correct
// while the real runtime is pending.
func (s *ONNXScorer) Score(features domain.Features) (float64, []domain.Factor, error) {
	// Pre-compute the feature vector to validate Features can be marshalled
	// into the on-disk model schema. If this panics we want to surface it
	// EARLY in tests, not at request time.
	_ = featuresToVector(features)

	// Placeholder factors with weight=0. Real ONNX SHAP path will fill these in.
	// Direction defaults to IncreasesRisk for skeleton — real impl computes
	// per-feature signed contribution and sets it dynamically.
	factors := []domain.Factor{
		{Name: "company_age_years", Value: features.CompanyAgeYears, Weight: 0,
			Direction: domain.IncreasesRisk},
		{Name: "has_blocked_okved", Value: features.HasBlockedOKVED, Weight: 0,
			Direction: domain.IncreasesRisk},
		{Name: "ubo_count", Value: features.UBOCount, Weight: 0,
			Direction: domain.IncreasesRisk},
		{Name: "has_foreign_owners", Value: features.HasForeignOwners, Weight: 0,
			Direction: domain.IncreasesRisk},
		{Name: "registration_address_is_mass", Value: features.RegistrationAddressIsMass,
			Weight: 0, Direction: domain.IncreasesRisk},
	}
	return 0, factors, ErrONNXNotWired
}

// ModelInfo returns the static metadata (read once in NewONNXScorer).
func (s *ONNXScorer) ModelInfo() domain.ModelInfo {
	return s.model
}

// featuresToVector returns a deterministic []float32 representation of
// Features using a fixed field order. This MUST match the column order
// used during CatBoost training — drift here = silent bad inferences.
//
// Order is contractual: extending Features with a new field requires
// retraining the model and bumping its version (per ADR-0011 promotion
// rules in docs/adr/0011-llm-routing-strategy.md).
func featuresToVector(f domain.Features) []float32 {
	return []float32{
		float32(f.CompanyAgeYears),
		boolToFloat(f.HasBlockedOKVED),
		boolToFloat(f.RegistrationAddressIsMass),
		float32(f.UBOCount),
		boolToFloat(f.HasForeignOwners),
		float32(f.CapitalKopecks),
	}
}

func boolToFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

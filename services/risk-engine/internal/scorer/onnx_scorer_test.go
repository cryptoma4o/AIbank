package scorer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
)

func TestNewONNXScorer_FileNotExists_Errors(t *testing.T) {
	t.Parallel()
	_, err := NewONNXScorer("/nonexistent/path/risk.onnx")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("error should mention stat: %v", err)
	}
}

func TestNewONNXScorer_BadExtension_Errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "risk.cbm")
	if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewONNXScorer(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported extension") {
		t.Fatalf("expected unsupported-extension error, got %v", err)
	}
}

func TestNewONNXScorer_EmptyPath_Errors(t *testing.T) {
	t.Parallel()
	_, err := NewONNXScorer("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNewONNXScorer_EmptyFile_Errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-v0.0.0.onnx")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewONNXScorer(path)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-file error, got %v", err)
	}
}

func TestNewONNXScorer_Success_ReadsMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Versioned filename: factory should pick this up as version.
	path := filepath.Join(dir, "risk-v1.2.0.onnx")
	if err := os.WriteFile(path, []byte("synthetic onnx bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewONNXScorer(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info := s.ModelInfo()
	if info.Name != "catboost-onboarding" {
		t.Errorf("name = %q, want catboost-onboarding", info.Name)
	}
	if info.Version != "risk-v1.2.0" {
		t.Errorf("version = %q, want risk-v1.2.0", info.Version)
	}
	if info.ComputedAt.IsZero() {
		t.Error("ComputedAt should be set at construction time")
	}
}

func TestNewONNXScorer_UnversionedFilename_FallsBackToChecksum(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(path, []byte("dummy bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewONNXScorer(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(s.ModelInfo().Version, "sha256:") {
		t.Errorf("version should fall back to sha256 prefix, got %q", s.ModelInfo().Version)
	}
}

func TestONNXScorer_Score_ReturnsNotWired(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-v1.0.0.onnx")
	if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewONNXScorer(path)
	if err != nil {
		t.Fatal(err)
	}
	score, factors, err := s.Score(domain.Features{
		CompanyAgeYears: 5,
		HasBlockedOKVED: false,
		UBOCount:        2,
	})
	if !errors.Is(err, ErrONNXNotWired) {
		t.Fatalf("expected ErrONNXNotWired, got %v", err)
	}
	if score != 0 {
		t.Errorf("score should be 0 placeholder, got %v", score)
	}
	if len(factors) != 5 {
		t.Errorf("expected 5 placeholder factors, got %d", len(factors))
	}
}

func TestFeaturesToVector_DeterministicOrder(t *testing.T) {
	t.Parallel()
	f := domain.Features{
		CompanyAgeYears:           3.5,
		HasBlockedOKVED:           true,
		RegistrationAddressIsMass: false,
		UBOCount:                  2,
		HasForeignOwners:          true,
		CapitalKopecks:            1000000,
	}
	v := featuresToVector(f)
	if len(v) != 6 {
		t.Fatalf("expected 6 features in vector, got %d", len(v))
	}
	if v[0] != 3.5 {
		t.Errorf("v[0] should be CompanyAgeYears=3.5, got %v", v[0])
	}
	if v[1] != 1 {
		t.Errorf("v[1] should be HasBlockedOKVED=1, got %v", v[1])
	}
	if v[2] != 0 {
		t.Errorf("v[2] should be RegistrationAddressIsMass=0, got %v", v[2])
	}
	// Re-call must produce identical bytes — contractual invariant for ML reproducibility.
	v2 := featuresToVector(f)
	for i := range v {
		if v[i] != v2[i] {
			t.Fatalf("featuresToVector is not deterministic at index %d", i)
		}
	}
}

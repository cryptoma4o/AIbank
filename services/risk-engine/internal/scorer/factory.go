package scorer

import (
	"log/slog"
	"os"
	"strings"
)

// Backend selects the scorer implementation. Defaults to "stub" if env not set.
type Backend string

const (
	// BackendStub is the deterministic SHA-256-based scorer (always available).
	BackendStub Backend = "stub"
	// BackendONNX wires real CatBoost via onnxruntime-go (Phase 2).
	BackendONNX Backend = "onnx"

	envBackend   = "SCORER"
	envModelPath = "RISK_MODEL_PATH"
)

// BuildScorer returns a CatBoostScorer based on env config:
//
//	SCORER=stub        (default): StubScorer
//	SCORER=onnx        + RISK_MODEL_PATH=<path>.onnx: ONNXScorer
//
// On ONNX init error (missing file, bad extension, etc) the factory falls
// back to StubScorer with a warn log so the service starts even with
// misconfigured model paths in dev. Production should treat the warn as
// blocking via alert.
//
// Logger is optional — pass slog.Default() if nil.
func BuildScorer(log *slog.Logger) CatBoostScorer {
	if log == nil {
		log = slog.Default()
	}
	backend := Backend(strings.ToLower(strings.TrimSpace(os.Getenv(envBackend))))
	switch backend {
	case BackendONNX:
		path := strings.TrimSpace(os.Getenv(envModelPath))
		if path == "" {
			log.Warn("scorer: SCORER=onnx but RISK_MODEL_PATH is empty; falling back to stub")
			return NewStubScorer()
		}
		s, err := NewONNXScorer(path)
		if err != nil {
			log.Warn("scorer: ONNX init failed; falling back to stub",
				"path", path, "err", err)
			return NewStubScorer()
		}
		log.Info("scorer: ONNX backend selected (TODO real inference, see onnx_scorer.go)",
			"path", path, "version", s.ModelInfo().Version)
		return s
	case BackendStub, "":
		log.Info("scorer: stub backend selected (deterministic SHA-256)")
		return NewStubScorer()
	default:
		log.Warn("scorer: unknown SCORER backend, falling back to stub", "value", string(backend))
		return NewStubScorer()
	}
}

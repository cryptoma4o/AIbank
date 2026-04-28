package scorer

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildScorer_DefaultIsStub(t *testing.T) {
	t.Setenv(envBackend, "")
	t.Setenv(envModelPath, "")
	s := BuildScorer(silentLogger())
	if _, ok := s.(*StubScorer); !ok {
		t.Errorf("expected *StubScorer, got %T", s)
	}
}

func TestBuildScorer_StubExplicit(t *testing.T) {
	t.Setenv(envBackend, "stub")
	s := BuildScorer(silentLogger())
	if _, ok := s.(*StubScorer); !ok {
		t.Errorf("expected *StubScorer, got %T", s)
	}
}

func TestBuildScorer_ONNXSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-v2.0.0.onnx")
	if err := os.WriteFile(path, []byte("dummy onnx"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envBackend, "onnx")
	t.Setenv(envModelPath, path)
	s := BuildScorer(silentLogger())
	onnx, ok := s.(*ONNXScorer)
	if !ok {
		t.Fatalf("expected *ONNXScorer, got %T", s)
	}
	if onnx.ModelInfo().Version != "risk-v2.0.0" {
		t.Errorf("version = %q, want risk-v2.0.0", onnx.ModelInfo().Version)
	}
}

func TestBuildScorer_ONNXMissingPath_FallsBackToStub(t *testing.T) {
	t.Setenv(envBackend, "onnx")
	t.Setenv(envModelPath, "")
	s := BuildScorer(silentLogger())
	if _, ok := s.(*StubScorer); !ok {
		t.Errorf("expected fallback *StubScorer, got %T", s)
	}
}

func TestBuildScorer_ONNXBadFile_FallsBackToStub(t *testing.T) {
	t.Setenv(envBackend, "onnx")
	t.Setenv(envModelPath, "/nonexistent/model.onnx")
	s := BuildScorer(silentLogger())
	if _, ok := s.(*StubScorer); !ok {
		t.Errorf("expected fallback *StubScorer, got %T", s)
	}
}

func TestBuildScorer_UnknownBackend_FallsBackToStub(t *testing.T) {
	t.Setenv(envBackend, "garbage")
	s := BuildScorer(silentLogger())
	if _, ok := s.(*StubScorer); !ok {
		t.Errorf("expected fallback *StubScorer, got %T", s)
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	ed25519signer "github.com/aibank/platform/packages/signature/ed25519"
	"github.com/aibank/platform/services/audit-service/internal/domain"
)

// memoryRepo — простой in-memory store для unit-тестов handler.
type memoryRepo struct {
	events []*domain.AuditEvent
}

func (r *memoryRepo) Append(_ context.Context, e *domain.AuditEvent) error {
	r.events = append(r.events, e)
	return nil
}

func (r *memoryRepo) LatestHash(_ context.Context, tenantID string) (string, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].TenantID == tenantID {
			return r.events[i].Hash, nil
		}
	}
	return "", nil
}

func (r *memoryRepo) List(_ context.Context, tenantID, _, _ string, limit int) ([]*domain.AuditEvent, error) {
	out := make([]*domain.AuditEvent, 0)
	for _, e := range r.events {
		if e.TenantID == tenantID {
			out = append(out, e)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRecordEvent_WithoutSigner — backwards compat: handler без signer
// продолжает работать, signature-поля остаются пустыми.
func TestRecordEvent_WithoutSigner(t *testing.T) {
	t.Parallel()
	repo := &memoryRepo{}
	h := NewEventHandler(repo, quietLogger())

	body := `{
		"tenant_id":"tnt_demo","entity_type":"application","entity_id":"app_1",
		"event_type":"submitted","actor_id":"per_42","actor_type":"user",
		"payload":{"channel":"web"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.RecordEvent(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(repo.events) != 1 {
		t.Fatalf("events stored = %d, want 1", len(repo.events))
	}
	saved := repo.events[0]
	if saved.HasSignature() {
		t.Errorf("event without signer should NOT have signature")
	}
	if saved.Hash == "" {
		t.Errorf("hash should be computed even without signer")
	}
}

// TestRecordEvent_WithSigner — handler подписывает событие, signature
// заполняется и проверяется.
func TestRecordEvent_WithSigner(t *testing.T) {
	t.Parallel()
	signer, err := ed25519signer.Generate()
	if err != nil {
		t.Fatalf("Generate signer: %v", err)
	}
	repo := &memoryRepo{}
	h := NewEventHandlerWithSigner(repo, quietLogger(), signer, ed25519signer.Algorithm)

	body := `{
		"tenant_id":"tnt_demo","entity_type":"application","entity_id":"app_1",
		"event_type":"approved","actor_id":"per_admin","actor_type":"user",
		"payload":{"by":"manual_review"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.RecordEvent(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	saved := repo.events[0]
	if !saved.HasSignature() {
		t.Fatalf("expected signature on event, got empty: %+v", saved)
	}
	if saved.SignatureAlgorithm != ed25519signer.Algorithm {
		t.Errorf("algorithm = %q, want %q", saved.SignatureAlgorithm, ed25519signer.Algorithm)
	}
	if saved.SignerKeyID != signer.KeyID() {
		t.Errorf("signer_key_id = %q, want %q", saved.SignerKeyID, signer.KeyID())
	}

	// Полная end-to-end верификация: signer.Verify должен принять подпись
	// над SignedDigest исходного события.
	digest, err := saved.SignedDigest()
	if err != nil {
		t.Fatalf("SignedDigest: %v", err)
	}
	if err := signer.Verify(digest, saved.Signature); err != nil {
		t.Errorf("Verify: %v — подпись не валидна, hash chain или signing сломаны", err)
	}
}

// TestRecordEvent_TwoEvents_HashChainIntact — два события подряд
// формируют валидную hash chain даже когда signer включён.
func TestRecordEvent_TwoEvents_HashChainIntact(t *testing.T) {
	t.Parallel()
	signer, _ := ed25519signer.Generate()
	repo := &memoryRepo{}
	h := NewEventHandlerWithSigner(repo, quietLogger(), signer, ed25519signer.Algorithm)

	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(map[string]any{
			"tenant_id":   "tnt_demo",
			"entity_type": "application",
			"entity_id":   "app_1",
			"event_type":  "step_" + string(rune('a'+i)),
			"actor_id":    "per_42",
			"actor_type":  "user",
			"payload":     map[string]int{"i": i},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/events/", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.RecordEvent(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("step %d failed: %s", i, w.Body.String())
		}
	}

	if len(repo.events) != 2 {
		t.Fatalf("events count = %d, want 2", len(repo.events))
	}
	first := repo.events[0]
	second := repo.events[1]

	if second.PreviousHash != first.Hash {
		t.Errorf("second.previous_hash = %q, want first.hash = %q",
			second.PreviousHash, first.Hash)
	}
	// Обе подписи должны верифицироваться независимо.
	for i, ev := range repo.events {
		digest, _ := ev.SignedDigest()
		if err := signer.Verify(digest, ev.Signature); err != nil {
			t.Errorf("event %d signature invalid: %v", i, err)
		}
	}
}

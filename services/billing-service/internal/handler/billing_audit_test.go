package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/billing-service/internal/domain"
)

// ─── Audit-server mock + client ──────────────────────────────────────────

func auditServer(t *testing.T) (*httptest.Server, *atomic.Int32, chan auditsdk.RecordEventRequest) {
	t.Helper()
	ch := make(chan auditsdk.RecordEventRequest, 8)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var req auditsdk.RecordEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case ch <- req:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(auditsdk.AuditEvent{
			ID: "evt_test", TenantID: req.TenantID, EventType: req.EventType,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, ch
}

func newTestAuditClient(t *testing.T, srv *httptest.Server) *auditsdk.Client {
	t.Helper()
	c, err := auditsdk.NewClient(auditsdk.ClientOptions{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Timeout:    1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// newHandlerWithAudit — EventHandler с in-memory fake-репо и заданным audit-client'ом.
func newHandlerWithAudit(t *testing.T, ac *auditsdk.Client) *EventHandler {
	t.Helper()
	repo := newFakeRepo()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEventHandler(repo, fixedTier{tier: domain.TierBasic}, ac, log)
}

// ─── Tests ───────────────────────────────────────────────────────────────

// TestAuditEmit_OnRecord — POST /v1/events успешен → "billing_event.recorded".
func TestAuditEmit_OnRecord(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h := newHandlerWithAudit(t, ac)

	const tenantID = "bank-alpha"
	body, _ := json.Marshal(map[string]any{
		"tenant_id":       tenantID,
		"event_type":      domain.EventTypeAccountOpenedLLC,
		"source_service":  "onboarding-orchestrator",
		"source_event_id": "audit-app-001",
	})

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-ID", "billing_op_7")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var stored domain.BillingEvent
	if err := json.Unmarshal(rr.Body.Bytes(), &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "billing_event.recorded" {
			t.Errorf("event_type = %q, want billing_event.recorded", ev.EventType)
		}
		if ev.TenantID != tenantID {
			t.Errorf("tenant_id = %q, want %q", ev.TenantID, tenantID)
		}
		if ev.EntityID != stored.ID {
			t.Errorf("entity_id = %q, want %q (billing_event.id)", ev.EntityID, stored.ID)
		}
		if ev.EntityType != "billing_event" {
			t.Errorf("entity_type = %q, want billing_event", ev.EntityType)
		}
		if ev.ActorID != "billing_op_7" {
			t.Errorf("actor_id = %q, want billing_op_7 (X-Actor-ID)", ev.ActorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_NoEmitOnValidationError — POST с пустым event_type → 400 → audit НЕ вызван.
func TestAuditEmit_NoEmitOnValidationError(t *testing.T) {
	srv, hits, _ := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h := newHandlerWithAudit(t, ac)

	// event_type пуст → recordEventRequest.validate вернёт ошибку → 400.
	body, _ := json.Marshal(map[string]any{
		"tenant_id":       "bank-alpha",
		"event_type":      "",
		"source_service":  "onboarding-orchestrator",
		"source_event_id": "x-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	time.Sleep(150 * time.Millisecond)
	if got := hits.Load(); got != 0 {
		t.Errorf("audit hits = %d, want 0 on 400", got)
	}
}

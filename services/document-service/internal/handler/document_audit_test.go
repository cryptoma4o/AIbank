package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"github.com/aibank/platform/services/document-service/internal/domain"
	"github.com/aibank/platform/services/document-service/internal/storage"
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

// newHandlerWithAudit собирает DocumentHandler с in-memory зависимостями + указанный audit-client.
func newHandlerWithAudit(t *testing.T, ac *auditsdk.Client) *DocumentHandler {
	t.Helper()
	repo := newFakeRepo()
	st := storage.NewInMemoryStorage()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewDocumentHandler(repo, st, ac, log)
}

// ─── Tests ───────────────────────────────────────────────────────────────

// TestAuditEmit_OnUpload — POST /v1/documents (multipart) → "document.uploaded".
func TestAuditEmit_OnUpload(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h := newHandlerWithAudit(t, ac)

	const tenantID = "bank_alpha"
	body, contentType := buildMultipart(t,
		map[string]string{
			"tenant_id":      tenantID,
			"application_id": "app_audit_1",
			"type":           "passport",
		},
		"file", "passport.pdf", "application/pdf",
		[]byte("%PDF-1.4 minimal pdf body for audit test"),
	)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Actor-ID", "operator_42")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var doc domain.Document
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "document.uploaded" {
			t.Errorf("event_type = %q, want document.uploaded", ev.EventType)
		}
		if ev.TenantID != tenantID {
			t.Errorf("tenant_id = %q, want %q", ev.TenantID, tenantID)
		}
		if ev.EntityID != doc.ID {
			t.Errorf("entity_id = %q, want %q (document.id)", ev.EntityID, doc.ID)
		}
		if ev.EntityType != "document" {
			t.Errorf("entity_type = %q, want document", ev.EntityType)
		}
		if ev.ActorID != "operator_42" {
			t.Errorf("actor_id = %q, want operator_42 (X-Actor-ID)", ev.ActorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_NoEmitOn400 — multipart без tenant_id → 400 → audit НЕ вызван.
func TestAuditEmit_NoEmitOn400(t *testing.T) {
	srv, hits, _ := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h := newHandlerWithAudit(t, ac)

	body, contentType := buildMultipart(t,
		map[string]string{
			// без tenant_id — handler ответит 400.
			"application_id": "app_audit_1",
			"type":           "passport",
		},
		"file", "passport.pdf", "application/pdf",
		[]byte("body"),
	)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Дать middleware шанс выстрелить (он не должен).
	time.Sleep(150 * time.Millisecond)
	if got := hits.Load(); got != 0 {
		t.Errorf("audit hits = %d, want 0 on 400", got)
	}
}


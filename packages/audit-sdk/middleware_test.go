package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newAuditServer captures incoming Append calls into a slice for assertions.
func newAuditServer(t *testing.T) (*httptest.Server, *atomic.Int32, chan RecordEventRequest) {
	t.Helper()
	ch := make(chan RecordEventRequest, 8)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var req RecordEventRequest
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
		_ = json.NewEncoder(w).Encode(AuditEvent{
			ID:        "evt_test",
			TenantID:  req.TenantID,
			EventType: req.EventType,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, ch
}

// makeMW wires a Client + EmitOnSuccess for a fixed event type and resolver.
func makeMW(
	t *testing.T,
	srv *httptest.Server,
	eventType string,
	resolve EntityResolver,
) func(http.Handler) http.Handler {
	t.Helper()
	c, err := NewClient(ClientOptions{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Timeout:    1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return EmitOnSuccess(c, eventType, resolve, slog.Default())
}

func TestEmitOnSuccess_EmitsAfter2xx(t *testing.T) {
	srv, hits, ch := newAuditServer(t)

	resolve := func(r *http.Request) (string, string, json.RawMessage, bool) {
		return "tenant", "bank_alpha", json.RawMessage(`{"foo":"bar"}`), true
	}
	mw := makeMW(t, srv, "tenant.created", resolve)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", nil)
	ctx := WithAuthInfo(req.Context(), AuthInfo{
		TenantID: "bank_alpha", ActorID: "user_1", ActorType: ActorTypeUser,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(ctx))

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d", rr.Code)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "tenant.created" || ev.TenantID != "bank_alpha" {
			t.Errorf("unexpected event: %+v", ev)
		}
		if ev.ActorID != "user_1" || ev.ActorType != ActorTypeUser {
			t.Errorf("unexpected actor: %+v", ev)
		}
		if string(ev.Payload) != `{"foo":"bar"}` {
			t.Errorf("payload = %s", ev.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("audit-service was not called within 1s")
	}
	if hits.Load() == 0 {
		t.Errorf("expected at least 1 hit")
	}
}

func TestEmitOnSuccess_DoesNotEmitOn4xx(t *testing.T) {
	srv, hits, ch := newAuditServer(t)
	resolve := func(r *http.Request) (string, string, json.RawMessage, bool) {
		return "tenant", "x", nil, true
	}
	mw := makeMW(t, srv, "tenant.created", resolve)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", nil)
	ctx := WithAuthInfo(req.Context(), AuthInfo{
		TenantID: "bank_alpha", ActorID: "u", ActorType: ActorTypeUser,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(ctx))

	// Give the goroutine a chance, but it shouldn't fire.
	select {
	case ev := <-ch:
		t.Fatalf("unexpected emit: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
	if hits.Load() != 0 {
		t.Errorf("expected 0 hits, got %d", hits.Load())
	}
}

func TestEmitOnSuccess_DoesNotEmitOn5xx(t *testing.T) {
	srv, hits, ch := newAuditServer(t)
	resolve := func(r *http.Request) (string, string, json.RawMessage, bool) {
		return "tenant", "x", nil, true
	}
	mw := makeMW(t, srv, "tenant.created", resolve)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", nil)
	ctx := WithAuthInfo(req.Context(), AuthInfo{
		TenantID: "t", ActorID: "u", ActorType: ActorTypeUser,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(ctx))

	select {
	case ev := <-ch:
		t.Fatalf("unexpected emit on 5xx: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
	if hits.Load() != 0 {
		t.Errorf("expected 0 hits, got %d", hits.Load())
	}
}

func TestEmitOnSuccess_SkipsWhenNoAuthInfo(t *testing.T) {
	srv, hits, ch := newAuditServer(t)
	resolve := func(r *http.Request) (string, string, json.RawMessage, bool) {
		return "tenant", "x", nil, true
	}
	mw := makeMW(t, srv, "tenant.created", resolve)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	select {
	case ev := <-ch:
		t.Fatalf("unexpected emit without auth: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
	if hits.Load() != 0 {
		t.Errorf("expected 0 hits, got %d", hits.Load())
	}
}

func TestEmitOnSuccess_SkipsWhenResolverReturnsFalse(t *testing.T) {
	srv, hits, ch := newAuditServer(t)
	resolve := func(r *http.Request) (string, string, json.RawMessage, bool) {
		return "", "", nil, false
	}
	mw := makeMW(t, srv, "tenant.created", resolve)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/tenants", nil)
	ctx := WithAuthInfo(req.Context(), AuthInfo{
		TenantID: "t", ActorID: "u", ActorType: ActorTypeUser,
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(ctx))

	select {
	case ev := <-ch:
		t.Fatalf("unexpected emit when resolver=false: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
	if hits.Load() != 0 {
		t.Errorf("expected 0 hits, got %d", hits.Load())
	}
}

func TestAuthInfoRoundtrip(t *testing.T) {
	info := AuthInfo{TenantID: "t", ActorID: "a", ActorType: ActorTypeAIAgent}
	ctx := WithAuthInfo(context.Background(), info)
	got, ok := AuthInfoFromContext(ctx)
	if !ok {
		t.Fatal("not found")
	}
	if got != info {
		t.Errorf("got %+v, want %+v", got, info)
	}
	// Missing case
	if _, ok := AuthInfoFromContext(context.Background()); ok {
		t.Error("expected ok=false on empty ctx")
	}
}

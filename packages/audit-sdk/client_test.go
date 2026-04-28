package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient builds a Client pointed at the given httptest server with
// short retry settings so the test suite stays fast.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(ClientOptions{
		BaseURL:    srv.URL,
		Timeout:    2 * time.Second,
		HTTPClient: srv.Client(),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestNewClient_RequiresBaseURL(t *testing.T) {
	if _, err := NewClient(ClientOptions{}); err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
}

func TestAppend_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/events" {
			t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var req RecordEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.TenantID != "bank_alpha" || req.EventType != "application.created" {
			t.Errorf("unexpected request: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(AuditEvent{
			ID:        "evt_123",
			TenantID:  req.TenantID,
			EventType: req.EventType,
			Hash:      "deadbeef",
			CreatedAt: time.Now().UTC(),
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	out, err := c.Append(context.Background(), RecordEventRequest{
		TenantID:   "bank_alpha",
		EntityType: "application",
		EntityID:   "app_42",
		EventType:  "application.created",
		ActorID:    "user_7",
		ActorType:  ActorTypeUser,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if out.ID != "evt_123" || out.Hash != "deadbeef" {
		t.Errorf("unexpected event: %+v", out)
	}
}

func TestAppend_AuthorizationHeader(t *testing.T) {
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(AuditEvent{ID: "evt_1"})
	}))
	defer srv.Close()

	c, _ := NewClient(ClientOptions{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		APIKey:     "s3cret",
	})
	if _, err := c.Append(context.Background(), RecordEventRequest{
		TenantID: "t", EntityType: "e", EntityID: "id",
		EventType: "x", ActorID: "a", ActorType: ActorTypeSystem,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if seenAuth != "Bearer s3cret" {
		t.Errorf("Authorization header = %q, want Bearer s3cret", seenAuth)
	}
}

func TestAppend_ValidationFailure(t *testing.T) {
	c, _ := NewClient(ClientOptions{BaseURL: "http://example.invalid"})
	_, err := c.Append(context.Background(), RecordEventRequest{TenantID: ""})
	if err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Fatalf("expected tenant_id validation error, got %v", err)
	}
}

func TestAppend_4xxIsNotRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "validation_failed", "message": "bad"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Append(context.Background(), RecordEventRequest{
		TenantID: "t", EntityType: "e", EntityID: "id",
		EventType: "x", ActorID: "a", ActorType: ActorTypeSystem,
	})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 || apiErr.Code != "validation_failed" {
		t.Errorf("unexpected APIError: %+v", apiErr)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (no retries on 4xx)", got)
	}
}

func TestAppend_RetryOn503SucceedsOn3rdAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(AuditEvent{ID: "evt_ok"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	out, err := c.Append(context.Background(), RecordEventRequest{
		TenantID: "t", EntityType: "e", EntityID: "id",
		EventType: "x", ActorID: "a", ActorType: ActorTypeSystem,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if out.ID != "evt_ok" {
		t.Errorf("unexpected id %q", out.ID)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestAppend_RetryBudgetExhausted(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, _ := NewClient(ClientOptions{
		BaseURL:    srv.URL,
		Timeout:    500 * time.Millisecond,
		HTTPClient: srv.Client(),
		MaxRetries: 3,
	})
	_, err := c.Append(context.Background(), RecordEventRequest{
		TenantID: "t", EntityType: "e", EntityID: "id",
		EventType: "x", ActorID: "a", ActorType: ActorTypeSystem,
	})
	if err == nil {
		t.Fatal("expected error after retries")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d", apiErr.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got < 1 {
		t.Errorf("calls = %d, want >=1", got)
	}
}

func TestList_WithFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/events" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("tenant_id") != "bank_alpha" {
			t.Errorf("tenant_id = %q", q.Get("tenant_id"))
		}
		if q.Get("entity_type") != "application" {
			t.Errorf("entity_type = %q", q.Get("entity_type"))
		}
		if q.Get("entity_id") != "app_42" {
			t.Errorf("entity_id = %q", q.Get("entity_id"))
		}
		if q.Get("limit") != "50" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(listResponse{
			Items: []*AuditEvent{
				{ID: "evt_1", EventType: "application.created"},
				{ID: "evt_2", EventType: "application.updated"},
			},
			Count: 2,
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	items, err := c.List(context.Background(), QueryOptions{
		TenantID:   "bank_alpha",
		EntityType: "application",
		EntityID:   "app_42",
		Limit:      50,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 || items[0].ID != "evt_1" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestList_RequiresTenant(t *testing.T) {
	c, _ := NewClient(ClientOptions{BaseURL: "http://example.invalid"})
	if _, err := c.List(context.Background(), QueryOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAppend_TransportErrorRetried(t *testing.T) {
	// Server that closes the connection before responding triggers a
	// transport error, which the retry loop should treat as retryable.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Skip("hijacker not supported")
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(AuditEvent{ID: "evt_after_retry"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	out, err := c.Append(context.Background(), RecordEventRequest{
		TenantID: "t", EntityType: "e", EntityID: "id",
		EventType: "x", ActorID: "a", ActorType: ActorTypeSystem,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if out.ID != "evt_after_retry" {
		t.Errorf("unexpected id %q", out.ID)
	}
}

func TestCorrelationContext(t *testing.T) {
	id := NewCorrelationID()
	if id == "" {
		t.Fatal("empty id")
	}
	ctx := ContextWithCorrelation(context.Background(), id)
	if got := CorrelationFromContext(ctx); got != id {
		t.Errorf("got %q, want %q", got, id)
	}
	// Empty id should generate one.
	ctx2 := ContextWithCorrelation(context.Background(), "")
	if CorrelationFromContext(ctx2) == "" {
		t.Errorf("expected generated id")
	}
}

// Sanity: APIError.IsRetryable.
func TestAPIError_IsRetryable(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{500, true}, {502, true}, {503, true}, {429, true},
		{400, false}, {401, false}, {404, false}, {200, false},
	}
	for _, c := range cases {
		got := (&APIError{StatusCode: c.code}).IsRetryable()
		if got != c.want {
			t.Errorf("status %d: got %v, want %v", c.code, got, c.want)
		}
	}
}


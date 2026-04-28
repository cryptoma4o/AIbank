package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"aibank/billing-service/internal/domain"
	"aibank/billing-service/internal/repository"
)

// fakeRepo — in-memory реализация BillingEventRepository, в т.ч. идемпотентного Append.
type fakeRepo struct {
	mu     sync.Mutex
	events map[string]*domain.BillingEvent
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{events: make(map[string]*domain.BillingEvent)}
}

func (f *fakeRepo) Append(_ context.Context, evt *domain.BillingEvent) (*domain.BillingEvent, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.events[evt.ID]; ok {
		return existing, false, nil
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	if evt.Quantity <= 0 {
		evt.Quantity = 1
	}
	evt.TotalKopecks = domain.ComputeTotal(evt.Quantity, evt.UnitPriceKopecks)
	stored := *evt
	f.events[evt.ID] = &stored
	return &stored, true, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (*domain.BillingEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if evt, ok := f.events[id]; ok {
		return evt, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepo) ListByTenant(
	_ context.Context, tenantID string, from, to time.Time, _ int,
) ([]*domain.BillingEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.BillingEvent, 0)
	for _, e := range f.events {
		if e.TenantID != tenantID {
			continue
		}
		if e.CreatedAt.Before(from) || !e.CreatedAt.Before(to) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeRepo) AggregateByTenant(
	_ context.Context, tenantID string, from, to time.Time,
) (*domain.Aggregate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	agg := &domain.Aggregate{
		TenantID:    tenantID,
		From:        from,
		To:          to,
		ByEventType: make(map[string]int64),
	}
	for _, e := range f.events {
		if e.TenantID != tenantID {
			continue
		}
		if e.CreatedAt.Before(from) || !e.CreatedAt.Before(to) {
			continue
		}
		agg.ByEventType[e.EventType] += e.TotalKopecks
		agg.TotalKopecks += e.TotalKopecks
		agg.EventCount++
	}
	return agg, nil
}

type fixedTier struct{ tier domain.Tier }

func (f fixedTier) ResolveTier(_ context.Context, _ string) domain.Tier { return f.tier }

func newTestHandler(t *testing.T, tier domain.Tier) (*EventHandler, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEventHandler(repo, fixedTier{tier: tier}, nil, log), repo
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func TestRecordEvent_CreatesNew(t *testing.T) {
	t.Parallel()

	h, repo := newTestHandler(t, domain.TierBasic)
	body := mustJSON(t, map[string]any{
		"tenant_id":       "bank-alpha",
		"event_type":      domain.EventTypeAccountOpenedLLC,
		"source_service":  "onboarding-orchestrator",
		"source_event_id": "app-001",
	})

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	var got domain.BillingEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.UnitPriceKopecks != 80000 {
		t.Errorf("expected basic LLC price 80000, got %d", got.UnitPriceKopecks)
	}
	if got.TotalKopecks != 80000 {
		t.Errorf("expected total 80000, got %d", got.TotalKopecks)
	}
	if len(repo.events) != 1 {
		t.Errorf("expected 1 event stored, got %d", len(repo.events))
	}
}

func TestRecordEvent_IdempotentSameSourceID(t *testing.T) {
	t.Parallel()

	h, repo := newTestHandler(t, domain.TierBasic)
	body := mustJSON(t, map[string]any{
		"tenant_id":       "bank-alpha",
		"event_type":      domain.EventTypeUBOCheckExecuted,
		"source_service":  "agent-ubo-tracing",
		"source_event_id": "ubo-42",
	})

	// Первый вызов — 201.
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first call: expected 201, got %d", rec.Code)
	}
	var first domain.BillingEvent
	_ = json.Unmarshal(rec.Body.Bytes(), &first)

	// Второй вызов с теми же {tenant_id, event_type, source_event_id} — 200 + та же запись.
	req2 := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200 (idempotent), got %d body=%s",
			rec2.Code, rec2.Body.String())
	}
	var second domain.BillingEvent
	_ = json.Unmarshal(rec2.Body.Bytes(), &second)
	if first.ID != second.ID {
		t.Errorf("expected same ID, got %q vs %q", first.ID, second.ID)
	}
	if len(repo.events) != 1 {
		t.Errorf("expected 1 event stored, got %d", len(repo.events))
	}
}

func TestGetEvent_RoundTrip(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, domain.TierEnterprise)
	body := mustJSON(t, map[string]any{
		"tenant_id":       "bank-alpha",
		"event_type":      domain.EventTypeAccountOpenedIP,
		"source_service":  "onboarding-orchestrator",
		"source_event_id": "ip-42",
	})

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	var created domain.BillingEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/events/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	h.Routes().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var fetched domain.BillingEvent
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, fetched.ID)
	}
	// Enterprise tier для IP — 40000 (400 ₽).
	if fetched.UnitPriceKopecks != 40000 {
		t.Errorf("expected enterprise IP price 40000, got %d", fetched.UnitPriceKopecks)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t, domain.TierBasic)

	req := httptest.NewRequest(http.MethodGet, "/events/nonexistent-id", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAggregate_SumsByEventType(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t, domain.TierBasic)

	post := func(eventType, sourceID string) {
		body := mustJSON(t, map[string]any{
			"tenant_id":       "bank-alpha",
			"event_type":      eventType,
			"source_service":  "onboarding-orchestrator",
			"source_event_id": sourceID,
		})
		req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
			t.Fatalf("post %s/%s: code %d", eventType, sourceID, rec.Code)
		}
	}

	post(domain.EventTypeAccountOpenedLLC, "a-1") // 80000
	post(domain.EventTypeAccountOpenedLLC, "a-2") // 80000
	post(domain.EventTypeAccountOpenedIP, "i-1")  // 20000
	post(domain.EventTypeUBOCheckExecuted, "u-1") // 15000

	from := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02")
	to := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")

	req := httptest.NewRequest(http.MethodGet,
		"/aggregate?tenant_id=bank-alpha&from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var agg domain.Aggregate
	if err := json.Unmarshal(rec.Body.Bytes(), &agg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	const wantTotal = 80000 + 80000 + 20000 + 15000
	if agg.TotalKopecks != wantTotal {
		t.Errorf("total = %d, want %d", agg.TotalKopecks, wantTotal)
	}
	if agg.EventCount != 4 {
		t.Errorf("event count = %d, want 4", agg.EventCount)
	}
	if agg.ByEventType[domain.EventTypeAccountOpenedLLC] != 160000 {
		t.Errorf("LLC sum = %d, want 160000", agg.ByEventType[domain.EventTypeAccountOpenedLLC])
	}
	if agg.ByEventType[domain.EventTypeUBOCheckExecuted] != 15000 {
		t.Errorf("UBO sum = %d, want 15000", agg.ByEventType[domain.EventTypeUBOCheckExecuted])
	}
}

func TestRecordEvent_ValidationErrors(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(t, domain.TierBasic)

	cases := []map[string]any{
		{"event_type": "x", "source_service": "s", "source_event_id": "e"}, // missing tenant_id
		{"tenant_id": "x", "source_service": "s", "source_event_id": "e"},  // missing event_type
		{"tenant_id": "x", "event_type": "y", "source_event_id": "e"},      // missing source_service
		{"tenant_id": "x", "event_type": "y", "source_service": "s"},       // missing source_event_id
	}
	for i, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/events",
			bytes.NewReader(mustJSON(t, body)))
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: expected 400, got %d", i, rec.Code)
		}
	}
}

// Sanity: убедиться, что fake repo действительно возвращает ErrNotFound,
// иначе тест GetEvent_NotFound будет давать ложно-зелёный сигнал.
func TestFakeRepoNotFoundIsErrNotFound(t *testing.T) {
	t.Parallel()
	r := newFakeRepo()
	_, err := r.GetByID(context.Background(), "missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

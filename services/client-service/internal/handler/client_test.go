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

	"aibank/client-service/internal/domain"
)

// --- in-memory stubs --------------------------------------------------------

type memClientRepo struct {
	mu      sync.Mutex
	byID    map[string]*domain.Client
	tenants map[string][]*domain.Client // ordered by insertion
}

func newMemClientRepo() *memClientRepo {
	return &memClientRepo{byID: map[string]*domain.Client{}, tenants: map[string][]*domain.Client{}}
}

func (m *memClientRepo) Create(_ context.Context, c *domain.Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *c
	m.byID[c.ID] = &cp
	m.tenants[c.TenantID] = append([]*domain.Client{&cp}, m.tenants[c.TenantID]...)
	return nil
}

func (m *memClientRepo) GetByID(_ context.Context, tenantID, id string) (*domain.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byID[id]
	if !ok || c.TenantID != tenantID {
		return nil, errors.New("not found")
	}
	cp := *c
	return &cp, nil
}

func (m *memClientRepo) ListByTenant(_ context.Context, tenantID string, limit, offset int) ([]*domain.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.tenants[tenantID]
	if offset >= len(all) {
		return []*domain.Client{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	out := make([]*domain.Client, 0, end-offset)
	for _, c := range all[offset:end] {
		cp := *c
		out = append(out, &cp)
	}
	return out, nil
}

func (m *memClientRepo) UpdateStatus(_ context.Context, tenantID, id string, status domain.ClientStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byID[id]
	if !ok || c.TenantID != tenantID {
		return errors.New("not found")
	}
	c.Status = status
	return nil
}

type memHistoryRepo struct {
	mu       sync.Mutex
	byClient map[string][]*domain.ClientHistory
}

func newMemHistoryRepo() *memHistoryRepo {
	return &memHistoryRepo{byClient: map[string][]*domain.ClientHistory{}}
}

func (m *memHistoryRepo) Append(_ context.Context, h *domain.ClientHistory, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *h
	m.byClient[h.ClientID] = append([]*domain.ClientHistory{&cp}, m.byClient[h.ClientID]...)
	return nil
}

func (m *memHistoryRepo) ListByClient(_ context.Context, _, clientID string, limit, offset int) ([]*domain.ClientHistory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.byClient[clientID]
	if offset >= len(all) {
		return []*domain.ClientHistory{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	out := make([]*domain.ClientHistory, 0, end-offset)
	for _, h := range all[offset:end] {
		cp := *h
		out = append(out, &cp)
	}
	return out, nil
}

// --- helpers ----------------------------------------------------------------

func newTestServer(t *testing.T) (*httptest.Server, *memClientRepo, *memHistoryRepo) {
	t.Helper()
	clients := newMemClientRepo()
	history := newMemHistoryRepo()
	log := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	h := NewClientHandler(clients, history, nil, log)
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, clients, history
}

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// --- tests ------------------------------------------------------------------

func TestCreateGetListClient(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)

	// Create
	resp := doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"tenant_id":       "alfa",
		"applicant_id":    "per_42",
		"legal_entity_id": "le_42",
		"status":          "onboarding",
		"risk_category":   "LOW",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	created := decode[domain.Client](t, resp)
	if created.ID == "" || created.Status != domain.ClientStatusOnboarding {
		t.Fatalf("unexpected created client: %+v", created)
	}

	// Get
	resp = doJSON(t, http.MethodGet, srv.URL+"/"+created.ID+"?tenant_id=alfa", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d", resp.StatusCode)
	}
	got := decode[domain.Client](t, resp)
	if got.ID != created.ID {
		t.Fatalf("got id %q, want %q", got.ID, created.ID)
	}

	// List
	resp = doJSON(t, http.MethodGet, srv.URL+"/?tenant_id=alfa", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", resp.StatusCode)
	}
	listed := decode[struct {
		Items []domain.Client `json:"items"`
		Count int             `json:"count"`
	}](t, resp)
	if listed.Count != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected list response: %+v", listed)
	}
}

func TestCreateClientValidation(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)

	// missing tenant_id
	resp := doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"applicant_id":    "per_1",
		"legal_entity_id": "le_1",
		"status":          "onboarding",
		"risk_category":   "LOW",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing tenant_id, got %d", resp.StatusCode)
	}

	// invalid status
	resp = doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"tenant_id":       "alfa",
		"applicant_id":    "per_1",
		"legal_entity_id": "le_1",
		"status":          "weird",
		"risk_category":   "LOW",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d", resp.StatusCode)
	}

	// invalid risk_category
	resp = doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"tenant_id":       "alfa",
		"applicant_id":    "per_1",
		"legal_entity_id": "le_1",
		"status":          "onboarding",
		"risk_category":   "X",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid risk_category, got %d", resp.StatusCode)
	}
}

func TestUpdateStatusAppendsHistory(t *testing.T) {
	t.Parallel()
	srv, clients, history := newTestServer(t)

	// seed via Create
	resp := doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"tenant_id":       "alfa",
		"applicant_id":    "per_1",
		"legal_entity_id": "le_1",
		"status":          "onboarding",
		"risk_category":   "MEDIUM",
	})
	created := decode[domain.Client](t, resp)

	// PATCH status
	resp = doJSON(t, http.MethodPatch, srv.URL+"/"+created.ID+"/status?tenant_id=alfa",
		map[string]any{"status": "active", "reason": "kyc passed"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on patch, got %d", resp.StatusCode)
	}

	got, err := clients.GetByID(context.Background(), "alfa", created.ID)
	if err != nil {
		t.Fatalf("repo get: %v", err)
	}
	if got.Status != domain.ClientStatusActive {
		t.Fatalf("status not applied, got %q", got.Status)
	}

	if len(history.byClient[created.ID]) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history.byClient[created.ID]))
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)
	resp := doJSON(t, http.MethodPatch, srv.URL+"/cli_missing/status?tenant_id=alfa",
		map[string]any{"status": "active"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAppendAndListHistory(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/", map[string]any{
		"tenant_id":       "alfa",
		"applicant_id":    "per_1",
		"legal_entity_id": "le_1",
		"status":          "onboarding",
		"risk_category":   "LOW",
	})
	created := decode[domain.Client](t, resp)

	for _, summary := range []string{"first", "second", "third"} {
		resp := doJSON(t, http.MethodPost, srv.URL+"/"+created.ID+"/history?tenant_id=alfa",
			map[string]any{"event_type": "test.event", "summary": summary, "source": "evt_1"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 for history, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/"+created.ID+"/history?tenant_id=alfa", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on history list, got %d", resp.StatusCode)
	}
	out := decode[struct {
		Items []domain.ClientHistory `json:"items"`
		Count int                    `json:"count"`
	}](t, resp)
	if out.Count != 3 {
		t.Fatalf("expected 3 history items, got %d", out.Count)
	}
	// newest first
	if out.Items[0].Summary != "third" {
		t.Fatalf("expected newest 'third' first, got %q", out.Items[0].Summary)
	}
}

func TestPaginationGuards(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)
	cases := []struct{ url string }{
		{srv.URL + "/?tenant_id=alfa&limit=0"},
		{srv.URL + "/?tenant_id=alfa&limit=500"},
		{srv.URL + "/?tenant_id=alfa&offset=-1"},
		{srv.URL + "/?tenant_id=alfa&limit=abc"},
	}
	for _, tc := range cases {
		resp := doJSON(t, http.MethodGet, tc.url, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", tc.url, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestMissingTenantID(t *testing.T) {
	t.Parallel()
	srv, _, _ := newTestServer(t)

	endpoints := []struct{ method, url string }{
		{http.MethodGet, srv.URL + "/cli_x"},
		{http.MethodGet, srv.URL + "/"},
		{http.MethodPatch, srv.URL + "/cli_x/status"},
		{http.MethodGet, srv.URL + "/cli_x/history"},
		{http.MethodPost, srv.URL + "/cli_x/history"},
	}
	for _, ep := range endpoints {
		body := any(nil)
		if ep.method == http.MethodPatch || ep.method == http.MethodPost {
			body = map[string]any{"status": "active", "event_type": "x", "summary": "y"}
		}
		resp := doJSON(t, ep.method, ep.url, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s %s: expected 400, got %d", ep.method, ep.url, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

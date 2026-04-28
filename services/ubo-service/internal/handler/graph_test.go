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
	"sort"
	"sync"
	"testing"

	"aibank/ubo-service/internal/domain"
)

// --- in-memory repo ---------------------------------------------------------

type memGraphRepo struct {
	mu    sync.Mutex
	byID  map[string]*domain.UBOGraph
	byLE  map[string][]*domain.UBOGraph // key = tenantID + "/" + legalEntityID
}

func newMemGraphRepo() *memGraphRepo {
	return &memGraphRepo{byID: map[string]*domain.UBOGraph{}, byLE: map[string][]*domain.UBOGraph{}}
}

func leKey(tenantID, leID string) string { return tenantID + "/" + leID }

func (m *memGraphRepo) Create(_ context.Context, g *domain.UBOGraph) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := 0
	for _, prev := range m.byLE[leKey(g.TenantID, g.LegalEntityID)] {
		if prev.Version > max {
			max = prev.Version
		}
	}
	g.Version = max + 1
	cp := *g
	m.byID[g.ID] = &cp
	m.byLE[leKey(g.TenantID, g.LegalEntityID)] = append(m.byLE[leKey(g.TenantID, g.LegalEntityID)], &cp)
	return nil
}

func (m *memGraphRepo) GetByID(_ context.Context, tenantID, id string) (*domain.UBOGraph, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.byID[id]
	if !ok || g.TenantID != tenantID {
		return nil, errors.New("not found")
	}
	cp := *g
	return &cp, nil
}

func (m *memGraphRepo) GetLatestByLegalEntity(_ context.Context, tenantID, leID string) (*domain.UBOGraph, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	versions := m.byLE[leKey(tenantID, leID)]
	if len(versions) == 0 {
		return nil, errors.New("not found")
	}
	sorted := make([]*domain.UBOGraph, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version > sorted[j].Version })
	cp := *sorted[0]
	return &cp, nil
}

func (m *memGraphRepo) ListByLegalEntity(_ context.Context, tenantID, leID string, limit, offset int) ([]*domain.UBOGraph, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	versions := m.byLE[leKey(tenantID, leID)]
	sorted := make([]*domain.UBOGraph, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version > sorted[j].Version })
	if offset >= len(sorted) {
		return []*domain.UBOGraph{}, nil
	}
	end := offset + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	out := make([]*domain.UBOGraph, 0, end-offset)
	for _, g := range sorted[offset:end] {
		cp := *g
		out = append(out, &cp)
	}
	return out, nil
}

// --- helpers ----------------------------------------------------------------

func newTestServer(t *testing.T) (*httptest.Server, *memGraphRepo) {
	t.Helper()
	repo := newMemGraphRepo()
	log := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	h := NewGraphHandler(repo, nil, log)
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, repo
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

// samplePayload — shape ровно как в agent-ubo-tracing/models/schemas.py:
// nodes:[{id,type,name,inn?}], edges:[{from,to,share_percent}],
// ubos:[{person_id,name,effective_share_percent,control_basis,paths}],
// confidence:0..1, unresolved_branches:[string].
func samplePayload(tenant, le string) map[string]any {
	return map[string]any{
		"tenant_id":       tenant,
		"legal_entity_id": le,
		"nodes": []map[string]any{
			{"id": "node-1", "type": "legal_entity", "name": "ООО Ромашка", "inn": "7701234567"},
			{"id": "node-2", "type": "person", "name": "Иванов И.И."},
		},
		"edges": []map[string]any{
			{"from": "node-2", "to": "node-1", "share_percent": 60.0},
		},
		"ubos": []map[string]any{
			{
				"person_id":               "per_42",
				"name":                    "Иванов И.И.",
				"effective_share_percent": 60.0,
				"control_basis":           "ownership",
				"paths":                   [][]string{{"node-2", "node-1"}},
			},
		},
		"confidence":          0.92,
		"unresolved_branches": []string{},
		"computed_by":         "agent-ubo-tracing v1.3",
	}
}

// --- tests ------------------------------------------------------------------

func TestCreateAndGetLatest(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/", samplePayload("alfa", "le_42"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	created := decode[domain.UBOGraph](t, resp)
	if created.Version != 1 {
		t.Fatalf("expected version=1 for first snapshot, got %d", created.Version)
	}
	if created.ID == "" {
		t.Fatalf("expected id to be assigned")
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/latest?tenant_id=alfa&legal_entity_id=le_42", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 latest, got %d", resp.StatusCode)
	}
	latest := decode[domain.UBOGraph](t, resp)
	if latest.ID != created.ID {
		t.Fatalf("latest id mismatch: %q vs %q", latest.ID, created.ID)
	}
}

func TestVersionIncrementsAndListOrdered(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)

	for i := 0; i < 3; i++ {
		resp := doJSON(t, http.MethodPost, srv.URL+"/", samplePayload("alfa", "le_42"))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("iter %d: expected 201, got %d", i, resp.StatusCode)
		}
		got := decode[domain.UBOGraph](t, resp)
		if got.Version != i+1 {
			t.Fatalf("iter %d: expected version=%d, got %d", i, i+1, got.Version)
		}
	}

	resp := doJSON(t, http.MethodGet, srv.URL+"/latest?tenant_id=alfa&legal_entity_id=le_42", nil)
	latest := decode[domain.UBOGraph](t, resp)
	if latest.Version != 3 {
		t.Fatalf("expected latest version=3, got %d", latest.Version)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/?tenant_id=alfa&legal_entity_id=le_42", nil)
	out := decode[struct {
		Items []domain.UBOGraph `json:"items"`
		Count int               `json:"count"`
	}](t, resp)
	if out.Count != 3 {
		t.Fatalf("expected 3 versions, got %d", out.Count)
	}
	// ordered desc
	if !(out.Items[0].Version > out.Items[1].Version &&
		out.Items[1].Version > out.Items[2].Version) {
		t.Fatalf("expected versions desc, got %d/%d/%d",
			out.Items[0].Version, out.Items[1].Version, out.Items[2].Version)
	}
}

func TestGetLatestNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/latest?tenant_id=alfa&legal_entity_id=le_missing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	cases := []map[string]any{
		// missing tenant_id
		{"legal_entity_id": "le_1", "computed_by": "x", "confidence": 0.5},
		// missing legal_entity_id
		{"tenant_id": "alfa", "computed_by": "x", "confidence": 0.5},
		// missing computed_by
		{"tenant_id": "alfa", "legal_entity_id": "le_1", "confidence": 0.5},
		// confidence out of range
		{"tenant_id": "alfa", "legal_entity_id": "le_1", "computed_by": "x", "confidence": 1.5},
	}
	for i, body := range cases {
		resp := doJSON(t, http.MethodPost, srv.URL+"/", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("case %d: expected 400, got %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestGetByID(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/", samplePayload("alfa", "le_42"))
	created := decode[domain.UBOGraph](t, resp)

	resp = doJSON(t, http.MethodGet, srv.URL+"/"+created.ID+"?tenant_id=alfa", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	got := decode[domain.UBOGraph](t, resp)
	if got.ID != created.ID {
		t.Fatalf("id mismatch")
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/ubg_missing?tenant_id=alfa", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPaginationGuards(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	cases := []string{
		"?tenant_id=alfa&legal_entity_id=le_1&limit=0",
		"?tenant_id=alfa&legal_entity_id=le_1&limit=500",
		"?tenant_id=alfa&legal_entity_id=le_1&offset=-1",
	}
	for _, qs := range cases {
		resp := doJSON(t, http.MethodGet, srv.URL+"/"+qs, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", qs, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestMissingQueryParams(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t)
	urls := []string{
		"/latest",                          // missing tenant_id+legal_entity_id
		"/latest?tenant_id=alfa",            // missing legal_entity_id
		"/?tenant_id=alfa",                  // missing legal_entity_id
		"/ubg_x",                           // missing tenant_id
	}
	for _, u := range urls {
		resp := doJSON(t, http.MethodGet, srv.URL+u, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", u, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

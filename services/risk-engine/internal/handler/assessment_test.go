package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aibank/platform/services/risk-engine/internal/domain"
	"github.com/aibank/platform/services/risk-engine/internal/handler"
)

// inMemoryRepo — minimal RiskAssessmentRepository for httptest.
type inMemoryRepo struct {
	mu    sync.Mutex
	items map[string]*domain.RiskAssessment
}

func newRepo() *inMemoryRepo { return &inMemoryRepo{items: map[string]*domain.RiskAssessment{}} }

func (r *inMemoryRepo) Create(_ context.Context, a *domain.RiskAssessment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[a.ID] = a
	return nil
}
func (r *inMemoryRepo) GetByID(_ context.Context, tenantID, id string) (*domain.RiskAssessment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.items[id]
	if !ok || a.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	return a, nil
}
func (r *inMemoryRepo) ListByApplication(_ context.Context, tenantID, appID string) ([]*domain.RiskAssessment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.RiskAssessment, 0)
	for _, a := range r.items {
		if a.TenantID == tenantID && a.ApplicationID == appID {
			out = append(out, a)
		}
	}
	return out, nil
}

// fakePipeline records calls and returns a fixed assessment via the repo.
type fakePipeline struct {
	repo  *inMemoryRepo
	count int
	err   error
}

func (f *fakePipeline) Assess(ctx context.Context, tenantID, applicationID string, _ map[string]any) (*domain.RiskAssessment, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.count++
	a := &domain.RiskAssessment{
		ID:             "ra_test_1",
		TenantID:       tenantID,
		ApplicationID:  applicationID,
		Score:          0.12,
		Category:       domain.CategoryLow,
		Recommendation: domain.RecommendAutoApprove,
		Model:          domain.ModelInfo{Name: "stub", Version: "0", ComputedAt: time.Now().UTC()},
		Factors:        []domain.Factor{},
		RulesTriggered: []domain.RuleTriggered{},
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := f.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func newServer(t *testing.T, p handler.Pipeline, repo domain.RiskAssessmentRepository) http.Handler {
	t.Helper()
	h := handler.NewAssessmentHandler(p, repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Mount("/v1/assessments", h.Routes())
	return r
}

func TestPostCreate_Then_GetById(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	body := bytes.NewBufferString(`{"tenant_id":"alfa","application_id":"app_1","input_data":{"company_age_years":5}}`)
	resp, err := http.Post(srv.URL+"/v1/assessments/", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d, body=%s", resp.StatusCode, b)
	}
	var created domain.RiskAssessment
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Errorf("missing id in response")
	}
	if pipe.count != 1 {
		t.Errorf("expected 1 pipeline call, got %d", pipe.count)
	}

	resp2, err := http.Get(srv.URL + "/v1/assessments/" + created.ID + "?tenant_id=alfa")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("get: %d", resp2.StatusCode)
	}
	var got domain.RiskAssessment
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.ID != created.ID || got.TenantID != "alfa" {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, created)
	}
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/assessments/missing?tenant_id=alfa")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGet_RequiresTenant(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/assessments/whatever")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestList_ByApplication(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	for i := 0; i < 2; i++ {
		body := bytes.NewBufferString(`{"tenant_id":"alfa","application_id":"app_42","input_data":{}}`)
		resp, err := http.Post(srv.URL+"/v1/assessments/", "application/json", body)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		resp.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/v1/assessments/?tenant_id=alfa&application_id=app_42")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status: %d", resp.StatusCode)
	}
	var out struct {
		Items []*domain.RiskAssessment `json:"items"`
		Count int                      `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	// Both creates produce items in the repo (overwriting same ID is fine —
	// fakePipeline always uses ra_test_1 in this test). Just verify >=1.
	if out.Count < 1 {
		t.Errorf("expected at least one item, got %d", out.Count)
	}
}

func TestPost_ValidationErrors(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing tenant", `{"application_id":"app"}`, http.StatusBadRequest},
		{"missing app", `{"tenant_id":"alfa"}`, http.StatusBadRequest},
		{"invalid json", `not-json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// no t.Parallel here — the outer test owns the httptest server
			// and closes it on return; running subtests in parallel would
			// race with the close.
			resp, err := http.Post(srv.URL+"/v1/assessments/", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestPost_PipelineError(t *testing.T) {
	t.Parallel()

	repo := newRepo()
	pipe := &fakePipeline{repo: repo, err: errors.New("boom")}
	srv := httptest.NewServer(newServer(t, pipe, repo))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/assessments/", "application/json",
		strings.NewReader(`{"tenant_id":"alfa","application_id":"app"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", resp.StatusCode)
	}
}

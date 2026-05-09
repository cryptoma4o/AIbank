package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
)

type fakeScreeningRepo struct {
	mu    sync.Mutex
	items map[string]*domain.ScreeningResultSet
}

func newFakeScreeningRepo() *fakeScreeningRepo {
	return &fakeScreeningRepo{items: map[string]*domain.ScreeningResultSet{}}
}

func (f *fakeScreeningRepo) Upsert(_ context.Context, s *domain.ScreeningResultSet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[s.TenantID+":"+s.ApplicationID] = s
	return nil
}

func (f *fakeScreeningRepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.ScreeningResultSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.items[tenantID+":"+applicationID]; ok {
		return s, nil
	}
	return nil, repository.ErrNotFound
}

func newScreeningHandlerForTest() (http.Handler, *fakeScreeningRepo) {
	repo := newFakeScreeningRepo()
	h := NewScreeningHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postScreening(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestScreening_HappyPath(t *testing.T) {
	h, repo := newScreeningHandlerForTest()
	score := 85.0
	rr := postScreening(t, h, map[string]any{
		"tenant_id":      "demo",
		"application_id": "app_x",
		"sanctions_results": []map[string]any{
			{"list_name": "OFAC_SDN", "match_level": "no_match", "score": 0.0},
			{"list_name": "EU_CFSP", "match_level": "no_match", "score": 0.0},
		},
		"pep_results": []map[string]any{
			{"list_name": "WORLD_CHECK_PEP", "match_level": "no_match", "score": 0.0},
		},
		"okved_consistency_score": &score,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_x"); got == nil || len(got.SanctionsResults) != 2 {
		t.Fatalf("repo missing or wrong sanctions count: %+v", got)
	}
}

func TestScreening_BadMatchLevel(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	rr := postScreening(t, h, map[string]any{
		"tenant_id":      "demo",
		"application_id": "app_x",
		"sanctions_results": []map[string]any{
			{"list_name": "OFAC", "match_level": "maybe", "score": 0.5},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

func TestScreening_BadScoreRange(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	rr := postScreening(t, h, map[string]any{
		"tenant_id":      "demo",
		"application_id": "app_x",
		"sanctions_results": []map[string]any{
			{"list_name": "OFAC", "match_level": "match", "score": 1.5},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (score>1)", rr.Code)
	}
}

func TestScreening_BadAdverseCategory(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	rr := postScreening(t, h, map[string]any{
		"tenant_id":      "demo",
		"application_id": "app_x",
		"adverse_media_hits": []map[string]any{
			{"category": "alien", "title": "X", "url": "http://x", "published_at": "2020-01-01"},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

func TestScreening_BadOKVEDScore(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	score := 150.0
	rr := postScreening(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"okved_consistency_score": &score,
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (okved>100)", rr.Code)
	}
}

func TestScreening_GetByApplication(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	postScreening(t, h, map[string]any{
		"tenant_id":      "demo",
		"application_id": "app_y",
		"sanctions_results": []map[string]any{
			{"list_name": "OFAC", "match_level": "no_match", "score": 0.0},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_y?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got domain.ScreeningResultSet
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got.SanctionsResults) != 1 {
		t.Errorf("sanctions len=%d want 1", len(got.SanctionsResults))
	}
}

func TestScreening_GetNotFound(t *testing.T) {
	h, _ := newScreeningHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

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

type fakeActivityRepo struct {
	mu         sync.Mutex
	activities map[string]*domain.ApplicationActivity
}

func newFakeActivityRepo() *fakeActivityRepo {
	return &fakeActivityRepo{activities: map[string]*domain.ApplicationActivity{}}
}

func (f *fakeActivityRepo) Upsert(_ context.Context, a *domain.ApplicationActivity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activities[a.TenantID+":"+a.ApplicationID] = a
	return nil
}

func (f *fakeActivityRepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.ApplicationActivity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.activities[tenantID+":"+applicationID]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func newActivityHandlerForTest() (http.Handler, *fakeActivityRepo) {
	repo := newFakeActivityRepo()
	h := NewActivityHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postActivity(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestActivity_HappyPath(t *testing.T) {
	h, repo := newActivityHandlerForTest()
	rr := postActivity(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_a",
		"business_description": "Разработка ПО для банков",
		"business_category":    "medium_risk",
		"top_suppliers": []map[string]any{
			{"name": "Хостинг.ру", "country": "RU", "share_percent": 40, "relationship_type": "regular"},
		},
		"funds_source": map[string]any{"category": "revenue"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_a"); got == nil || got.BusinessCategory != "medium_risk" {
		t.Fatalf("repo missing activity: %+v", got)
	}
}

func TestActivity_BadCategory(t *testing.T) {
	h, _ := newActivityHandlerForTest()
	rr := postActivity(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_a",
		"business_description": "X",
		"business_category":    "extreme",
		"funds_source":         map[string]any{"category": "revenue"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad category)", rr.Code)
	}
}

func TestActivity_FundsOtherRequiresDescription(t *testing.T) {
	h, _ := newActivityHandlerForTest()
	rr := postActivity(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_a",
		"business_description": "X",
		"business_category":    "low_risk",
		"funds_source":         map[string]any{"category": "other"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (funds.other w/o description)", rr.Code)
	}
}

func TestActivity_TooManySuppliers(t *testing.T) {
	h, _ := newActivityHandlerForTest()
	suppliers := []map[string]any{}
	for i := 0; i < 6; i++ {
		suppliers = append(suppliers, map[string]any{
			"name": "X", "country": "RU", "share_percent": 1, "relationship_type": "regular",
		})
	}
	rr := postActivity(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_a",
		"business_description": "X",
		"business_category":    "low_risk",
		"top_suppliers":        suppliers,
		"funds_source":         map[string]any{"category": "revenue"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (>5 suppliers)", rr.Code)
	}
}

func TestActivity_BadCounterpartyShare(t *testing.T) {
	h, _ := newActivityHandlerForTest()
	rr := postActivity(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_a",
		"business_description": "X",
		"business_category":    "low_risk",
		"top_suppliers": []map[string]any{
			{"name": "X", "country": "RU", "share_percent": 150, "relationship_type": "regular"},
		},
		"funds_source": map[string]any{"category": "revenue"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (share>100)", rr.Code)
	}
}

func TestActivity_GetByApplication_NotFound(t *testing.T) {
	h, _ := newActivityHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

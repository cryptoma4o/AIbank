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

type fakeMonitoringRepo struct {
	mu       sync.Mutex
	profiles map[string]*domain.MonitoringProfile
}

func newFakeMonitoringRepo() *fakeMonitoringRepo {
	return &fakeMonitoringRepo{profiles: map[string]*domain.MonitoringProfile{}}
}

func (f *fakeMonitoringRepo) Upsert(_ context.Context, p *domain.MonitoringProfile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[p.TenantID+":"+p.ApplicationID] = p
	return nil
}

func (f *fakeMonitoringRepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.MonitoringProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.profiles[tenantID+":"+applicationID]; ok {
		return p, nil
	}
	return nil, repository.ErrNotFound
}

func newMonitoringHandlerForTest() (http.Handler, *fakeMonitoringRepo) {
	repo := newFakeMonitoringRepo()
	h := NewMonitoringHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postMonitoring(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestMonitoring_HappyPath(t *testing.T) {
	h, repo := newMonitoringHandlerForTest()
	rr := postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"account_id":              "acc_x",
		"review_frequency_months": 12,
		"next_review_date":        "2027-05-09",
		"monitoring_rules": []map[string]any{
			{"code": "375-P-2.7", "description": "Cash share monitoring", "params": map[string]any{"threshold_pct": 30}},
		},
		"kyc_refresh_triggers":  []string{"scheduled", "ceo_change"},
		"notification_channels": []string{"email", "siem"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_x"); got == nil || got.ReviewFrequencyMonths != 12 {
		t.Fatalf("repo wrong: %+v", got)
	}
}

func TestMonitoring_BadFrequency(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	rr := postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"review_frequency_months": 9,
		"next_review_date":        "2027-05-09",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (frequency must be 3|6|12)", rr.Code)
	}
}

func TestMonitoring_BadDate(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	rr := postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"review_frequency_months": 12,
		"next_review_date":        "09.05.2027",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad date format)", rr.Code)
	}
}

func TestMonitoring_BadTrigger(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	rr := postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"review_frequency_months": 6,
		"next_review_date":        "2027-05-09",
		"kyc_refresh_triggers":    []string{"alien_invasion"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad trigger)", rr.Code)
	}
}

func TestMonitoring_BadRule(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	rr := postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"review_frequency_months": 6,
		"next_review_date":        "2027-05-09",
		"monitoring_rules": []map[string]any{
			{"code": "", "description": "x"},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (rule code required)", rr.Code)
	}
}

func TestMonitoring_GetByApplication(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	postMonitoring(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_y",
		"review_frequency_months": 3,
		"next_review_date":        "2026-08-09",
	})
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_y?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got domain.MonitoringProfile
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.ReviewFrequencyMonths != 3 {
		t.Errorf("frequency=%d want 3", got.ReviewFrequencyMonths)
	}
}

func TestMonitoring_GetNotFound(t *testing.T) {
	h, _ := newMonitoringHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

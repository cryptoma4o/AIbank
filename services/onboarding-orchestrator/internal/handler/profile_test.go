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

// fakeProfileRepo — управляемый stub под LegalEntityProfileRepository.
type fakeProfileRepo struct {
	mu       sync.Mutex
	profiles map[string]*domain.LegalEntityProfile // ключ = tenantID + ":" + applicationID
}

func newFakeProfileRepo() *fakeProfileRepo {
	return &fakeProfileRepo{profiles: map[string]*domain.LegalEntityProfile{}}
}

func (f *fakeProfileRepo) Upsert(_ context.Context, p *domain.LegalEntityProfile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[p.TenantID+":"+p.ApplicationID] = p
	return nil
}

func (f *fakeProfileRepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.LegalEntityProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.profiles[tenantID+":"+applicationID]; ok {
		return p, nil
	}
	return nil, repository.ErrNotFound
}

func newProfileHandlerForTest() (http.Handler, *fakeProfileRepo) {
	repo := newFakeProfileRepo()
	h := NewProfileHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postProfile(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestProfile_HappyPath — upsert валидной анкеты возвращает 200 + сохранённое тело.
func TestProfile_HappyPath(t *testing.T) {
	h, repo := newProfileHandlerForTest()
	rr := postProfile(t, h, map[string]any{
		"tenant_id":         "demo",
		"application_id":    "app_x",
		"legal_entity_id":   "le_x",
		"opf_code":          "12300",
		"okved_main_v2":     "62.01",
		"legal_address_struct": map[string]any{"country_code": "RU", "city": "Москва"},
		"contacts":          map[string]any{"phone": "+79001112233", "email": "info@test.ru"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_x"); got == nil || got.OPFCode != "12300" {
		t.Fatalf("repo missing profile: %+v", got)
	}
}

// TestProfile_BadOKVED — невалидный код ОКВЭД отклоняется.
func TestProfile_BadOKVED(t *testing.T) {
	h, _ := newProfileHandlerForTest()
	rr := postProfile(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"okved_main_v2":   "ABCDE",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

// TestProfile_BadAddress — legal_address без city отклоняется.
func TestProfile_BadAddress(t *testing.T) {
	h, _ := newProfileHandlerForTest()
	rr := postProfile(t, h, map[string]any{
		"tenant_id":            "demo",
		"application_id":       "app_x",
		"legal_entity_id":      "le_x",
		"legal_address_struct": map[string]any{"country_code": "RU"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

// TestProfile_BadLicense — license без issuer отклоняется.
func TestProfile_BadLicense(t *testing.T) {
	h, _ := newProfileHandlerForTest()
	rr := postProfile(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"licenses":        []map[string]any{{"number": "L-1", "issue_date": "2020-01-01"}},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

// TestProfile_GetByApplication — после upsert GET возвращает сохранённое.
func TestProfile_GetByApplication(t *testing.T) {
	h, _ := newProfileHandlerForTest()
	postProfile(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_y",
		"legal_entity_id": "le_y",
		"opf_code":        "12300",
	})
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_y?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got domain.LegalEntityProfile
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.OPFCode != "12300" {
		t.Errorf("opf_code = %q want 12300", got.OPFCode)
	}
}

// TestProfile_Get_NotFound — 404 для несуществующего application_id.
func TestProfile_Get_NotFound(t *testing.T) {
	h, _ := newProfileHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

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

type fakeRepRepo struct {
	mu   sync.Mutex
	reps map[string]*domain.Representative // ключ = id
}

func newFakeRepRepo() *fakeRepRepo {
	return &fakeRepRepo{reps: map[string]*domain.Representative{}}
}

func (f *fakeRepRepo) Upsert(_ context.Context, r *domain.Representative) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reps[r.ID] = r
	return nil
}

func (f *fakeRepRepo) GetByID(_ context.Context, _, id string) (*domain.Representative, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.reps[id]; ok {
		return r, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepRepo) ListByApplication(_ context.Context, tenantID, applicationID string) ([]*domain.Representative, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.Representative, 0)
	for _, r := range f.reps {
		if r.TenantID == tenantID && r.ApplicationID == applicationID {
			out = append(out, r)
		}
	}
	return out, nil
}

func newRepHandlerForTest() (http.Handler, *fakeRepRepo) {
	repo := newFakeRepRepo()
	h := NewRepresentativeHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postRep(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func validRepBody(overrides map[string]any) map[string]any {
	body := map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"last_name":       "Иванов",
		"first_name":      "Дмитрий",
		"middle_name":     "Александрович",
		"birth_date":      "1980-05-15",
		"citizenship":     []string{"RU"},
		"id_document": map[string]any{
			"doc_type":   "passport_ru",
			"series":     "4500",
			"number":     "123456",
			"issue_date": "2010-06-01",
			"issued_by":  "ОВД Москвы",
		},
		"authority": map[string]any{
			"position":         "Генеральный директор",
			"authority_basis":  "charter",
		},
		"is_primary":   true,
		"is_signatory": true,
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

func TestRepresentative_HappyPath(t *testing.T) {
	h, repo := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if len(repo.reps) != 1 {
		t.Fatalf("want 1 rep, got %d", len(repo.reps))
	}
}

func TestRepresentative_BadBirthDate(t *testing.T) {
	h, _ := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(map[string]any{"birth_date": "15.05.1980"}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad birth_date)", rr.Code)
	}
}

func TestRepresentative_BadDocType(t *testing.T) {
	h, _ := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(map[string]any{
		"id_document": map[string]any{"doc_type": "driver_license", "number": "1"},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad doc_type)", rr.Code)
	}
}

func TestRepresentative_BadAuthorityBasis(t *testing.T) {
	h, _ := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(map[string]any{
		"authority": map[string]any{"position": "X", "authority_basis": "verbal"},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad authority_basis)", rr.Code)
	}
}

func TestRepresentative_PDLRequiresFields(t *testing.T) {
	h, _ := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(map[string]any{
		"pdl_declaration": map[string]any{"is_pdl": true},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (PDL=true requires category/relation/position)", rr.Code)
	}
}

func TestRepresentative_BadCitizenshipCode(t *testing.T) {
	h, _ := newRepHandlerForTest()
	rr := postRep(t, h, validRepBody(map[string]any{"citizenship": []string{"RUS"}}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (citizenship must be 2 letters)", rr.Code)
	}
}

func TestRepresentative_ListByApplication(t *testing.T) {
	h, _ := newRepHandlerForTest()
	postRep(t, h, validRepBody(map[string]any{"id": "rep_a"}))
	postRep(t, h, validRepBody(map[string]any{
		"id": "rep_b", "is_primary": false, "first_name": "Пётр",
	}))
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_x?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var resp struct {
		Items []domain.Representative `json:"items"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 reps, got %d", len(resp.Items))
	}
}

func TestRepresentative_GetByID_NotFound(t *testing.T) {
	h, _ := newRepHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

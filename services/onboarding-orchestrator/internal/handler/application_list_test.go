package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"aibank/onboarding-orchestrator/internal/domain"
)

// listTestSetup поднимает chi-роутер с реальным ApplicationHandler и
// возвращает клиент для запросов в тесте.
func listTestSetup(t *testing.T, seed []*domain.Application) http.Handler {
	t.Helper()
	repo := newFakeAppRepo()
	for _, a := range seed {
		_ = repo.Create(context.Background(), a)
	}
	h := NewApplicationHandler(repo, nil, fixedIDGen{id: "app_x"}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewMux()
	r.Mount("/v1/applications", h.Routes())
	return r
}

func mustGet(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// listResponse — ожидаемая форма ответа List.
// Формат `{items:[...]}` — контракт с bff-admin orchestrator client
// (services/bff-admin/internal/clients/orchestrator.go).
type listResponse struct {
	Items []domain.Application `json:"items"`
}

// TestList_HappyPath — два тенанта seeded; запрос с tenant_id=demo возвращает
// только записи demo (изоляция тенанта).
func TestList_HappyPath(t *testing.T) {
	apps := []*domain.Application{
		{ID: "app1", TenantID: "demo", ApplicantID: "a1", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateDraft},
		{ID: "app2", TenantID: "demo", ApplicantID: "a2", LegalEntityType: domain.LegalEntityIP, Channel: domain.ChannelWeb, State: domain.StateApproved},
		{ID: "app3", TenantID: "alpha", ApplicantID: "a3", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateDraft},
	}
	h := listTestSetup(t, apps)

	rr := mustGet(t, h, "/v1/applications?tenant_id=demo")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got listResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 apps for tenant demo, got %d", len(got.Items))
	}
	for _, a := range got.Items {
		if a.TenantID != "demo" {
			t.Fatalf("cross-tenant leak: %+v", a)
		}
	}
}

// TestList_StateFilter — фильтрация по state (in-memory в handler).
func TestList_StateFilter(t *testing.T) {
	apps := []*domain.Application{
		{ID: "a", TenantID: "demo", ApplicantID: "x", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateDraft},
		{ID: "b", TenantID: "demo", ApplicantID: "y", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateApproved},
	}
	h := listTestSetup(t, apps)

	rr := mustGet(t, h, "/v1/applications?tenant_id=demo&state=approved")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got listResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got.Items) != 1 || got.Items[0].ID != "b" {
		t.Fatalf("expected only id=b approved, got %+v", got.Items)
	}
}

// TestList_BadTenant — 400 на невалидный tenant_id (regex check).
func TestList_BadTenant(t *testing.T) {
	h := listTestSetup(t, nil)
	rr := mustGet(t, h, "/v1/applications?tenant_id=BAD-id!")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body)
	}
}

// TestList_EmptyTenantID — отсутствующий tenant_id тоже 400.
func TestList_EmptyTenantID(t *testing.T) {
	h := listTestSetup(t, nil)
	rr := mustGet(t, h, "/v1/applications")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TestList_ApplicantFilter — фильтрация по applicant_id защищает от
// information disclosure между applicants одного тенанта. Bff-onboarding
// передаёт UserID из JWT, чтобы applicant A не увидел заявки applicant B.
func TestList_ApplicantFilter(t *testing.T) {
	apps := []*domain.Application{
		{ID: "appA1", TenantID: "demo", ApplicantID: "userA", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateDraft},
		{ID: "appA2", TenantID: "demo", ApplicantID: "userA", LegalEntityType: domain.LegalEntityIP, Channel: domain.ChannelWeb, State: domain.StateApproved},
		{ID: "appB1", TenantID: "demo", ApplicantID: "userB", LegalEntityType: domain.LegalEntityLLC, Channel: domain.ChannelWeb, State: domain.StateDraft},
	}
	h := listTestSetup(t, apps)

	rr := mustGet(t, h, "/v1/applications?tenant_id=demo&applicant_id=userA")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got listResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 apps for userA, got %d: %+v", len(got.Items), got.Items)
	}
	for _, a := range got.Items {
		if a.ApplicantID != "userA" {
			t.Fatalf("cross-applicant leak: %+v", a)
		}
	}
}

// TestList_BadState — невалидное значение state.
func TestList_BadState(t *testing.T) {
	h := listTestSetup(t, nil)
	rr := mustGet(t, h, "/v1/applications?tenant_id=demo&state=not_a_real_state")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body)
	}
}

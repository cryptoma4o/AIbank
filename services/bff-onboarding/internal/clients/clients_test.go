package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTenantClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tenants/tnt_alpha" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"tnt_alpha","name":"Alpha","bik":"044525593","inn":"7728168971",
			"status":"active","deployment_mode":"saas"
		}`))
	}))
	defer srv.Close()

	c := NewTenantClient(srv.URL)
	got, err := c.GetByID(context.Background(), "tnt_alpha")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != "tnt_alpha" || got.Name != "Alpha" || got.Status != "active" {
		t.Fatalf("unexpected tenant: %+v", got)
	}
}

func TestTenantClient_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewTenantClient(srv.URL)
	_, err := c.GetByID(context.Background(), "tnt_missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTransport_RetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"transient"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"tnt_x","name":"X","bik":"044525593","inn":"7728168971","status":"active","deployment_mode":"saas"}`))
	}))
	defer srv.Close()

	c := NewTenantClient(srv.URL)
	got, err := c.GetByID(context.Background(), "tnt_x")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls (1 retry), got %d", calls)
	}
	if got.ID != "tnt_x" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestTransport_GivesUpAfterRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewTenantClient(srv.URL)
	_, err := c.GetByID(context.Background(), "tnt_x")
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
	// 1 + MaxRetries = 2 попытки.
	if atomic.LoadInt32(&calls) != int32(1+MaxRetries) {
		t.Fatalf("expected %d calls, got %d", 1+MaxRetries, calls)
	}
}

func TestDocumentClient_ListByApplication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant-Id") != "tnt_alpha" {
			t.Fatalf("missing tenant header: %v", r.Header)
		}
		if r.URL.Query().Get("application_id") != "app_1" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"doc_1","type":"PASSPORT","application_id":"app_1","filename":"p.pdf","state":"uploaded","uploaded_at":"2026-04-26T10:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := NewDocumentClient(srv.URL)
	docs, err := c.ListByApplication(context.Background(), "tnt_alpha", "app_1")
	if err != nil {
		t.Fatalf("ListByApplication: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "doc_1" || string(docs[0].Type) != "PASSPORT" {
		t.Fatalf("unexpected docs: %+v", docs)
	}
}

func TestOrchestratorClient_GetApplication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tenant_id") != "tnt_alpha" {
			t.Fatalf("expected tenant_id query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"app_1","tenant_id":"tnt_alpha","applicant_id":"app_2","legal_entity_type":"LLC",
			"channel":"web","state":"validating","product_codes":["current_rub"],"workflow_id":"wf_1",
			"created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"
		}`))
	}))
	defer srv.Close()

	c := NewOrchestratorClient(srv.URL)
	got, err := c.GetApplication(context.Background(), "tnt_alpha", "app_1")
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}
	if got.State != "validating" || got.LegalEntityType != "LLC" {
		t.Fatalf("unexpected app: %+v", got)
	}
}

// TestOrchestratorClient_ListByApplicant_FiltersByApplicant — security regression:
// applicant A не должен видеть заявки applicant B того же тенанта.
// Передаём applicant_id в query (server-side filter в orchestrator) И
// делаем client-side проверку (defense-in-depth) на случай старой версии
// orchestrator'а, которая проигнорирует параметр.
func TestOrchestratorClient_ListByApplicant_FiltersByApplicant(t *testing.T) {
	var gotApplicant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotApplicant = r.URL.Query().Get("applicant_id")
		if r.URL.Query().Get("tenant_id") != "tnt_alpha" {
			t.Fatalf("expected tenant_id=tnt_alpha, got %q", r.URL.Query().Get("tenant_id"))
		}
		// Симулируем broken orchestrator, который вернёт ВСЕ заявки тенанта,
		// включая чужие. Client-side фильтр обязан их отбросить.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"appA","tenant_id":"tnt_alpha","applicant_id":"userA","legal_entity_type":"LLC","channel":"web","state":"draft","product_codes":[],"workflow_id":"","created_at":"2026-05-01T10:00:00Z","updated_at":"2026-05-01T10:00:00Z"},
			{"id":"appB","tenant_id":"tnt_alpha","applicant_id":"userB","legal_entity_type":"LLC","channel":"web","state":"draft","product_codes":[],"workflow_id":"","created_at":"2026-05-01T10:00:00Z","updated_at":"2026-05-01T10:00:00Z"}
		]}`))
	}))
	defer srv.Close()

	c := NewOrchestratorClient(srv.URL)
	apps, err := c.ListByApplicant(context.Background(), "tnt_alpha", "userA")
	if err != nil {
		t.Fatalf("ListByApplicant: %v", err)
	}
	if gotApplicant != "userA" {
		t.Fatalf("expected server to receive applicant_id=userA, got %q", gotApplicant)
	}
	if len(apps) != 1 || apps[0].ID != "appA" || apps[0].ApplicantID != "userA" {
		t.Fatalf("cross-applicant leak: %+v", apps)
	}
}

// TestOrchestratorClient_ListByApplicant_EmptyApplicantRejected — пустой
// applicantID — пограничный случай: без фильтра клиент мог бы получить
// все заявки тенанта. Возвращаем ошибку, не делая HTTP-запрос.
func TestOrchestratorClient_ListByApplicant_EmptyApplicantRejected(t *testing.T) {
	c := NewOrchestratorClient("http://unused")
	if _, err := c.ListByApplicant(context.Background(), "tnt_alpha", ""); err == nil {
		t.Fatal("expected error for empty applicantID, got nil")
	}
}

func TestOrchestratorClient_SignalDocumentsUploaded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/applications/app_1/signals/documents-uploaded" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()

	c := NewOrchestratorClient(srv.URL)
	err := c.SignalDocumentsUploaded(context.Background(), "tnt_alpha", "app_1",
		[]map[string]any{{"document_id": "doc_1"}})
	if err != nil {
		t.Fatalf("Signal: %v", err)
	}
}

func TestRiskClient_NotFoundReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewRiskClient(srv.URL)
	ra, err := c.GetByApplication(context.Background(), "tnt_alpha", "app_1")
	if err != nil {
		t.Fatalf("expected nil error for 404, got %v", err)
	}
	if ra != nil {
		t.Fatalf("expected nil RiskAssessment, got %+v", ra)
	}
}

func TestRiskClient_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"risk_1","application_id":"app_1","score":0.42,"category":"MEDIUM",
			"recommendation":"MANUAL_REVIEW","computed_at":"2026-04-26T10:05:00Z"
		}`))
	}))
	defer srv.Close()

	c := NewRiskClient(srv.URL)
	ra, err := c.GetByApplication(context.Background(), "tnt_alpha", "app_1")
	if err != nil {
		t.Fatalf("GetByApplication: %v", err)
	}
	if ra == nil || ra.ID != "risk_1" || ra.Category != "MEDIUM" {
		t.Fatalf("unexpected: %+v", ra)
	}
	if ra.ComputedAt.IsZero() {
		t.Fatalf("expected non-zero ComputedAt, got %v", ra.ComputedAt)
	}
	_ = time.Now // keep import even if go vet complains
}

func TestIdentityClient_GetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-tok" {
			t.Fatalf("unexpected Authorization: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"u_1","tenant_id":"tnt_alpha","email":"a@b.c","role":"bank.operator","is_active":true
		}`))
	}))
	defer srv.Close()

	c := NewIdentityClient(srv.URL)
	me, err := c.GetMe(context.Background(), "access-tok")
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.UserID != "u_1" || me.Email != "a@b.c" || me.Role != "bank.operator" {
		t.Fatalf("unexpected me: %+v", me)
	}
}

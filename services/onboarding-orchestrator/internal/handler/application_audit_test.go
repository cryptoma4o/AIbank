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
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.temporal.io/sdk/client"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
	"aibank/onboarding-orchestrator/internal/workflow"
)

// ─── In-memory fakes ─────────────────────────────────────────────────────

// fakeAppRepo — потокобезопасный in-memory ApplicationRepository.
type fakeAppRepo struct {
	mu   sync.Mutex
	apps map[string]*domain.Application // ключ = tenantID + ":" + id
}

func newFakeAppRepo() *fakeAppRepo {
	return &fakeAppRepo{apps: map[string]*domain.Application{}}
}

func (f *fakeAppRepo) Create(_ context.Context, app *domain.Application) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.apps[app.TenantID+":"+app.ID] = app
	return nil
}

func (f *fakeAppRepo) GetByID(_ context.Context, tenantID, id string) (*domain.Application, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.apps[tenantID+":"+id]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeAppRepo) UpdateState(_ context.Context, _, _ string, _ domain.ApplicationState) error {
	return nil
}

func (f *fakeAppRepo) ListByTenant(_ context.Context, tenantID string, limit int) ([]*domain.Application, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domain.Application, 0, len(f.apps))
	for _, a := range f.apps {
		if a.TenantID == tenantID {
			out = append(out, a)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeAppRepo) GetByApplicant(_ context.Context, tenantID, applicantID string) (*domain.Application, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.apps {
		if a.TenantID == tenantID && a.ApplicantID == applicantID {
			return a, nil
		}
	}
	return nil, repository.ErrNotFound
}

// fixedIDGen — детерминированный генератор для тестов.
type fixedIDGen struct{ id string }

func (f fixedIDGen) NewApplicationID() string { return f.id }

// ─── Temporal client mock (interface narrowing) ──────────────────────────
//
// client.Client — большой интерфейс (30+ методов).  Мы используем только
// ExecuteWorkflow и SignalWorkflow.  Чтобы не реализовывать весь интерфейс,
// встраиваем client.Client как nil-интерфейс (все остальные методы паникуют
// при вызове, но компилятор удовлетворён).  В тестах мы вызываем ТОЛЬКО
// две перекрытые операции — поэтому panic'ов не будет.

type fakeWorkflowRun struct {
	id    string
	runID string
}

func (f *fakeWorkflowRun) GetID() string                                    { return f.id }
func (f *fakeWorkflowRun) GetRunID() string                                  { return f.runID }
func (f *fakeWorkflowRun) Get(_ context.Context, _ interface{}) error       { return nil }
func (f *fakeWorkflowRun) GetWithOptions(_ context.Context, _ interface{}, _ client.WorkflowRunGetOptions) error {
	return nil
}

// fakeTemporal реализует только то, что использует ApplicationHandler:
// ExecuteWorkflow и SignalWorkflow.  Остальные методы интерфейса
// проксируются на embedded nil — вызов панично завершит тест, что
// сразу укажет на регрессию контракта.
type fakeTemporal struct {
	client.Client // nil-embedding: остальные методы panic'нут при вызове

	mu               sync.Mutex
	executeCalls     int
	signalCalls      int
	lastSignalName   string
	lastSignalArg    interface{}
	executeErr       error
	signalErr        error
}

func (f *fakeTemporal) ExecuteWorkflow(_ context.Context, opts client.StartWorkflowOptions,
	_ interface{}, _ ...interface{},
) (client.WorkflowRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executeCalls++
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	return &fakeWorkflowRun{id: opts.ID, runID: "run_test"}, nil
}

func (f *fakeTemporal) SignalWorkflow(_ context.Context, _, _ string,
	signalName string, arg interface{},
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signalCalls++
	f.lastSignalName = signalName
	f.lastSignalArg = arg
	return f.signalErr
}

// ─── Audit-server mock + client ──────────────────────────────────────────

func auditServer(t *testing.T) (*httptest.Server, *atomic.Int32, chan auditsdk.RecordEventRequest) {
	t.Helper()
	ch := make(chan auditsdk.RecordEventRequest, 8)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var req auditsdk.RecordEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case ch <- req:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(auditsdk.AuditEvent{
			ID: "evt_test", TenantID: req.TenantID, EventType: req.EventType,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, ch
}

func newTestAuditClient(t *testing.T, srv *httptest.Server) *auditsdk.Client {
	t.Helper()
	c, err := auditsdk.NewClient(auditsdk.ClientOptions{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Timeout:    1 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// newHandlerForAuditTest собирает ApplicationHandler с in-memory fake'ами.
func newHandlerForAuditTest(t *testing.T, ac *auditsdk.Client) (*ApplicationHandler, *fakeAppRepo, *fakeTemporal) {
	t.Helper()
	repo := newFakeAppRepo()
	tc := &fakeTemporal{}
	idgen := fixedIDGen{id: "app_audit_001"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewApplicationHandler(repo, tc, idgen, ac, log), repo, tc
}

// ─── Tests ───────────────────────────────────────────────────────────────

// TestAuditEmit_OnCreate — POST /v1/applications успешен → "application.created".
func TestAuditEmit_OnCreate(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h, _, tc := newHandlerForAuditTest(t, ac)

	const tenantID = "bank_alpha"
	body, _ := json.Marshal(map[string]any{
		"tenant_id":         tenantID,
		"applicant_id":      "app_user_1",
		"legal_entity_type": domain.LegalEntityLLC,
		"channel":           domain.ChannelWeb,
		"product_codes":     []string{"current_account"},
		"risk_thresholds": map[string]float64{
			"auto_approve_below": 0.30,
			"decline_above":      0.85,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-ID", "operator_77")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if tc.executeCalls != 1 {
		t.Errorf("ExecuteWorkflow calls = %d, want 1", tc.executeCalls)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "application.created" {
			t.Errorf("event_type = %q, want application.created", ev.EventType)
		}
		if ev.TenantID != tenantID {
			t.Errorf("tenant_id = %q, want %q", ev.TenantID, tenantID)
		}
		if ev.EntityID != "app_audit_001" {
			t.Errorf("entity_id = %q, want app_audit_001", ev.EntityID)
		}
		if ev.EntityType != "application" {
			t.Errorf("entity_type = %q, want application", ev.EntityType)
		}
		if ev.ActorID != "operator_77" {
			t.Errorf("actor_id = %q, want operator_77", ev.ActorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_OnSignalDocumentsUploaded — POST signal → "application.signal_received".
func TestAuditEmit_OnSignalDocumentsUploaded(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h, repo, tc := newHandlerForAuditTest(t, ac)

	const tenantID = "bank_alpha"
	const appID = "app_signal_1"

	// Засеять заявку, иначе signal-handler ответит 404.
	if err := repo.Create(context.Background(), &domain.Application{
		ID:         appID,
		TenantID:   tenantID,
		WorkflowID: "wf_" + appID,
		State:      domain.StateDraft,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"tenant_id": tenantID,
		"documents": []map[string]string{{
			"id":          "doc_1",
			"type":        "passport",
			"storage_uri": "memory://documents/passport",
		}},
	})

	r := chi.NewRouter()
	r.Mount("/v1/applications", h.Routes())

	req := httptest.NewRequest(http.MethodPost,
		"/v1/applications/"+appID+"/signals/documents-uploaded",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-ID", "ops_user")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if tc.signalCalls != 1 {
		t.Errorf("SignalWorkflow calls = %d, want 1", tc.signalCalls)
	}
	if tc.lastSignalName != workflow.SignalDocumentsUploaded {
		t.Errorf("signal name = %q, want %q", tc.lastSignalName, workflow.SignalDocumentsUploaded)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "application.signal_received" {
			t.Errorf("event_type = %q, want application.signal_received", ev.EventType)
		}
		if ev.TenantID != tenantID {
			t.Errorf("tenant_id = %q, want %q", ev.TenantID, tenantID)
		}
		if ev.EntityID != appID {
			t.Errorf("entity_id = %q, want %q", ev.EntityID, appID)
		}
		if ev.EntityType != "application" {
			t.Errorf("entity_type = %q, want application", ev.EntityType)
		}
		// Проверяем, что в payload зафиксирован сам signal.
		if !bytes.Contains(ev.Payload, []byte(`"signal":"documents_uploaded"`)) {
			t.Errorf("payload missing signal=documents_uploaded: %s", string(ev.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_NoEmitOn4xx — POST /v1/applications с невалидным телом → 400 → audit НЕ вызван.
func TestAuditEmit_NoEmitOn4xx(t *testing.T) {
	srv, hits, _ := auditServer(t)
	ac := newTestAuditClient(t, srv)
	h, _, tc := newHandlerForAuditTest(t, ac)

	// невалидный tenant_id (uppercase) → 400.
	body, _ := json.Marshal(map[string]any{
		"tenant_id":         "BAD",
		"applicant_id":      "x",
		"legal_entity_type": domain.LegalEntityLLC,
		"channel":           domain.ChannelWeb,
		"product_codes":     []string{"x"},
		"risk_thresholds": map[string]float64{
			"auto_approve_below": 0.30,
			"decline_above":      0.85,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if tc.executeCalls != 0 {
		t.Errorf("ExecuteWorkflow calls = %d, want 0 on validation error", tc.executeCalls)
	}

	time.Sleep(150 * time.Millisecond)
	if got := hits.Load(); got != 0 {
		t.Errorf("audit hits = %d, want 0 on 400", got)
	}
}

// Sanity: fake-репо корректно возвращает ErrNotFound, иначе тесты получат ложный сигнал.
func TestFakeAppRepoNotFoundIsErrNotFound(t *testing.T) {
	t.Parallel()
	r := newFakeAppRepo()
	_, err := r.GetByID(context.Background(), "t", "missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

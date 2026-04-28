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
	"sync/atomic"
	"testing"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"github.com/aibank/platform/services/tenant-service/internal/domain"
)

// fakeTenantRepo — простая in-memory реализация для unit-тестов.
type fakeTenantRepo struct {
	mu      sync.Mutex
	tenants map[string]*domain.Tenant
}

func newFakeTenantRepo() *fakeTenantRepo {
	return &fakeTenantRepo{tenants: map[string]*domain.Tenant{}}
}

func (f *fakeTenantRepo) Create(_ context.Context, t *domain.Tenant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tenants[t.ID] = t
	return nil
}

func (f *fakeTenantRepo) GetByID(_ context.Context, id string) (*domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tenants[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (f *fakeTenantRepo) List(_ context.Context) ([]*domain.Tenant, error) {
	return nil, nil
}

func (f *fakeTenantRepo) Update(_ context.Context, _ *domain.Tenant) error {
	return nil
}

type fakeConfigRepo struct{}

func (fakeConfigRepo) GetConfig(_ context.Context, _ string) (*domain.TenantConfig, error) {
	return &domain.TenantConfig{}, nil
}

func (fakeConfigRepo) SaveConfig(_ context.Context, _ *domain.TenantConfig) error {
	return nil
}

type fakeProvisioner struct{}

func (fakeProvisioner) Provision(_ context.Context, _ string) error          { return nil }
func (fakeProvisioner) SchemaExists(_ context.Context, _ string) (bool, error) { return true, nil }

// auditServer — мок-аудит-сервис; собирает входящие запросы.
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

// TestAuditEmit_OnSuccessfulCreate — 2xx → audit-event отправлен.
func TestAuditEmit_OnSuccessfulCreate(t *testing.T) {
	srv, hits, ch := auditServer(t)
	auditClient := newTestAuditClient(t, srv)

	h := NewTenantHandler(newFakeTenantRepo(), fakeConfigRepo{}, fakeProvisioner{}, auditClient,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	router := h.Routes()

	body := []byte(`{"id":"bank_alpha","name":"Alpha","bik":"044525974","inn":"7700000000","deployment_mode":"saas"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Actor-ID", "admin_1")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	select {
	case ev := <-ch:
		if ev.EventType != "tenant.created" {
			t.Errorf("event_type = %q, want tenant.created", ev.EventType)
		}
		if ev.TenantID != "bank_alpha" {
			t.Errorf("tenant_id = %q, want bank_alpha", ev.TenantID)
		}
		if ev.ActorID != "admin_1" {
			t.Errorf("actor_id = %q, want admin_1", ev.ActorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_NoEmitOn4xx — 4xx → audit не вызывается.
func TestAuditEmit_NoEmitOn4xx(t *testing.T) {
	srv, hits, _ := auditServer(t)
	auditClient := newTestAuditClient(t, srv)

	h := NewTenantHandler(newFakeTenantRepo(), fakeConfigRepo{}, fakeProvisioner{}, auditClient,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	router := h.Routes()

	// Невалидный body → 400
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"id":""}`)))
	req.Header.Set("X-Actor-ID", "admin_1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Fatalf("expected 4xx, got %d", rr.Code)
	}

	// Дать middleware шанс выстрелить (он не должен).
	time.Sleep(100 * time.Millisecond)
	if got := hits.Load(); got != 0 {
		t.Errorf("audit hits = %d, want 0 on 4xx", got)
	}
}

// TestAuditEmit_NilClientNoPanic — отсутствие audit-клиента (DEV mode)
// не должно ломать handler-flow.
func TestAuditEmit_NilClientNoPanic(t *testing.T) {
	h := NewTenantHandler(newFakeTenantRepo(), fakeConfigRepo{}, fakeProvisioner{}, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := h.Routes()

	body := []byte(`{"id":"bank_beta","name":"Beta","bik":"044525974","inn":"7700000000","deployment_mode":"saas"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Actor-ID", "admin_1")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
}

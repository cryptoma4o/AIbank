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

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/identity-service/internal/auth"
	"aibank/identity-service/internal/domain"
	"aibank/identity-service/internal/repository"
)

// ─── In-memory fakes ─────────────────────────────────────────────────────

// fakeUserRepo — in-memory реализация domain.UserRepository.
type fakeUserRepo struct {
	mu       sync.Mutex
	byID     map[string]*domain.User
	byEmail  map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    map[string]*domain.User{},
		byEmail: map[string]*domain.User{},
	}
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[u.ID] = u
	f.byEmail[u.Email] = u
	return nil
}

// fakeApplicantRepo — in-memory реализация domain.ApplicantRepository.
type fakeApplicantRepo struct {
	mu         sync.Mutex
	byID       map[string]*domain.Applicant
	byTenantINN map[string]*domain.Applicant
}

func newFakeApplicantRepo() *fakeApplicantRepo {
	return &fakeApplicantRepo{
		byID:        map[string]*domain.Applicant{},
		byTenantINN: map[string]*domain.Applicant{},
	}
}

func (f *fakeApplicantRepo) GetByID(_ context.Context, _, id string) (*domain.Applicant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.byID[id]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeApplicantRepo) GetByINN(_ context.Context, tenantID, inn string) (*domain.Applicant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.byTenantINN[tenantID+":"+inn]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeApplicantRepo) Create(_ context.Context, a *domain.Applicant) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[a.ID] = a
	f.byTenantINN[a.TenantID+":"+a.INN] = a
	return nil
}

// CreateWithConsents — fake-реализация; consents игнорируются, потому
// что audit-тесты их не проверяют (см. consent_test.go для consent flow).
func (f *fakeApplicantRepo) CreateWithConsents(
	ctx context.Context, a *domain.Applicant, _ []domain.Consent,
) error {
	return f.Create(ctx, a)
}

// Forget — fake-реализация 152-ФЗ ст. 14: scrub PII, set forgotten_at.
func (f *fakeApplicantRepo) Forget(_ context.Context, _, id, requester string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.byID[id]; ok && a.ForgottenAt == nil {
		now := time.Now().UTC()
		a.ForgottenAt = &now
		a.ForgottenBy = requester
		a.FullName = ""
		a.Phone = ""
		a.INN = ""
		a.PassportSeries = ""
		a.PassportNumber = ""
		a.SNILS = ""
	}
	return nil
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

// newAuthHandlerForTest собирает AuthHandler с in-memory fake'ами + JWT issuer.
func newAuthHandlerForTest(t *testing.T, ac *auditsdk.Client) (*AuthHandler, *fakeUserRepo, *fakeApplicantRepo) {
	t.Helper()
	users := newFakeUserRepo()
	applicants := newFakeApplicantRepo()
	secret := bytes.Repeat([]byte("a"), 32)
	issuer, err := auth.NewIssuer(secret)
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	verifier, err := auth.NewVerifier(secret)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewAuthHandler(users, applicants, &noopConsentRepo{}, issuer, verifier, ac, log)
	return h, users, applicants
}

// noopConsentRepo — заглушка ConsentRepository для audit-тестов;
// все методы возвращают пустые результаты без ошибок.
type noopConsentRepo struct{}

func (noopConsentRepo) Record(context.Context, *domain.Consent) error { return nil }
func (noopConsentRepo) ListByApplicant(context.Context, string, string) ([]*domain.Consent, error) {
	return nil, nil
}
func (noopConsentRepo) Revoke(context.Context, string, string, domain.ConsentType) error {
	return nil
}

// seedActiveBankUser создаёт активного банковского оператора с tenant_id.
// tenant_id обязателен: middleware EmitOnSuccess пропускает emit при пустом tenant_id.
func seedActiveBankUser(t *testing.T, repo *fakeUserRepo, email, password, tenantID string) *domain.User {
	t.Helper()
	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	tid := tenantID
	u := &domain.User{
		ID:           "user_op_1",
		TenantID:     &tid,
		Email:        email,
		PasswordHash: hash,
		Role:         domain.RoleBankOperator,
		IsActive:     true,
	}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// ─── Tests ───────────────────────────────────────────────────────────────

// TestAuditEmit_OnLogin — POST /v1/auth/login успешен → audit-event "identity.login".
//
// NOTE: actor_id фиксируется в AuthInfo ДО запуска handler'а (см.
// emitOnSuccessFromHolder в audit_middleware.go).  rememberActor лишь
// обновляет holder, поэтому в эмите попадает значение, которое было
// в holder на момент входа в middleware — для public-эндпоинтов это
// "system".  entity_id (user.id) корректно подхватывается через
// auditsdk.SetEntity.  Этот тест документирует фактическое поведение —
// если оно изменится, корректировка spec обязательна.
func TestAuditEmit_OnLogin(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)

	h, users, _ := newAuthHandlerForTest(t, ac)
	const email = "operator@bank.example"
	const password = "SuperSecret123!"
	const tenantID = "bank_alpha"
	u := seedActiveBankUser(t, users, email, password, tenantID)

	body, _ := json.Marshal(map[string]string{
		"email":     email,
		"password":  password,
		"tenant_id": tenantID,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	select {
	case ev := <-ch:
		if ev.EventType != "identity.login" {
			t.Errorf("event_type = %q, want identity.login", ev.EventType)
		}
		if ev.TenantID != tenantID {
			t.Errorf("tenant_id = %q, want %q", ev.TenantID, tenantID)
		}
		if ev.EntityID != u.ID {
			t.Errorf("entity_id = %q, want %q", ev.EntityID, u.ID)
		}
		if ev.EntityType != "user" {
			t.Errorf("entity_type = %q, want user", ev.EntityType)
		}
		// actor_id для public-логина — "system" (см. NOTE выше).
		if ev.ActorID == "" {
			t.Errorf("actor_id must be non-empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_OnApplicantRegister — POST /v1/applicants → "applicant.registered".
//
// См. NOTE в TestAuditEmit_OnLogin: actor_id для public-эндпоинта —
// "system"; entity_id корректно равен applicant.ID.
func TestAuditEmit_OnApplicantRegister(t *testing.T) {
	srv, hits, ch := auditServer(t)
	ac := newTestAuditClient(t, srv)

	h, _, _ := newAuthHandlerForTest(t, ac)

	body, _ := json.Marshal(map[string]any{
		"tenant_id": "bank_alpha",
		"inn":       "123456789012",
		"phone":     "+79991234567",
		"full_name": "Иванов Иван Иванович",
		"consents": []map[string]any{
			{"type": "data_processing", "granted": true, "version": "v2026-04"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/applicants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var created domain.Applicant
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.EventType != "applicant.registered" {
			t.Errorf("event_type = %q, want applicant.registered", ev.EventType)
		}
		if ev.TenantID != "bank_alpha" {
			t.Errorf("tenant_id = %q, want bank_alpha", ev.TenantID)
		}
		if ev.EntityID != created.ID {
			t.Errorf("entity_id = %q, want %q", ev.EntityID, created.ID)
		}
		if ev.EntityType != "applicant" {
			t.Errorf("entity_type = %q, want applicant", ev.EntityType)
		}
		if ev.ActorID == "" {
			t.Errorf("actor_id must be non-empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("audit emit timed out (hits=%d)", hits.Load())
	}
}

// TestAuditEmit_NoEmitOn401 — неверный пароль → 401 → audit НЕ вызывается.
func TestAuditEmit_NoEmitOn401(t *testing.T) {
	srv, hits, _ := auditServer(t)
	ac := newTestAuditClient(t, srv)

	h, users, _ := newAuthHandlerForTest(t, ac)
	const email = "operator@bank.example"
	const tenantID = "bank_alpha"
	seedActiveBankUser(t, users, email, "CorrectPass456!", tenantID)

	body, _ := json.Marshal(map[string]string{
		"email":     email,
		"password":  "WrongPass789!",
		"tenant_id": tenantID,
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Дать middleware шанс выстрелить (он не должен).
	time.Sleep(150 * time.Millisecond)
	if got := hits.Load(); got != 0 {
		t.Errorf("audit hits = %d, want 0 on 401", got)
	}
}

// Sanity: fake возвращает ErrNotFound, а не nil.
func TestFakeUserRepoNotFoundIsErrNotFound(t *testing.T) {
	t.Parallel()
	r := newFakeUserRepo()
	_, err := r.GetByEmail(context.Background(), "nobody@example.com")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

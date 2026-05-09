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

type fakeAccountRepo struct {
	mu       sync.Mutex
	accounts map[string]*domain.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{accounts: map[string]*domain.Account{}}
}

func (f *fakeAccountRepo) Upsert(_ context.Context, a *domain.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[a.TenantID+":"+a.ApplicationID] = a
	return nil
}

func (f *fakeAccountRepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.accounts[tenantID+":"+applicationID]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func newAccountHandlerForTest() (http.Handler, *fakeAccountRepo) {
	repo := newFakeAccountRepo()
	h := NewAccountHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postAccount(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func validAccountBody(overrides map[string]any) map[string]any {
	body := map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"currency":        "RUB",
		"account_type":    "settlement",
		"agreements": map[string]any{
			"agreement_acceptance":   true,
			"agreement_accepted_at":  "2026-05-09T10:00:00Z",
			"personal_data_consent":  true,
			"signing_method":         "sms_code",
		},
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

func TestAccount_HappyPath(t *testing.T) {
	h, repo := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_x"); got == nil || got.AccountType != "settlement" {
		t.Fatalf("repo wrong: %+v", got)
	}
}

func TestAccount_BadCurrency(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{"currency": "RUBL"}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (currency 4 letters)", rr.Code)
	}
}

func TestAccount_BadAccountType(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{"account_type": "savings"}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad account_type)", rr.Code)
	}
}

func TestAccount_RequiresPDConsent(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{
		"agreements": map[string]any{
			"agreement_acceptance":   true,
			"agreement_accepted_at":  "2026-05-09T10:00:00Z",
			"personal_data_consent":  false, // 152-FZ violation
			"signing_method":         "sms_code",
		},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (personal_data_consent required by 152-FZ)", rr.Code)
	}
}

func TestAccount_UKEPRequiresSerial(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{
		"agreements": map[string]any{
			"agreement_acceptance":   true,
			"agreement_accepted_at":  "2026-05-09T10:00:00Z",
			"personal_data_consent":  true,
			"signing_method":         "ukep",
			// no ukep_certificate_serial
		},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (UKEP requires certificate_serial)", rr.Code)
	}
}

func TestAccount_DBORequiresChannels(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{
		"agreements": map[string]any{
			"agreement_acceptance":   true,
			"agreement_accepted_at":  "2026-05-09T10:00:00Z",
			"personal_data_consent":  true,
			"signing_method":         "sms_code",
			"dbo_agreement":          true,
			// no dbo_channels
		},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (dbo_agreement=true requires channels)", rr.Code)
	}
}

func TestAccount_BadAccountNumber(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{
		"account_number": "1234567890", // not 20 digits
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (account_number must be 20 digits)", rr.Code)
	}
}

func TestAccount_BadBIK(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	rr := postAccount(t, h, validAccountBody(map[string]any{
		"bik": "123",
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bik must be 9 digits)", rr.Code)
	}
}

func TestAccount_GetByApplication(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	postAccount(t, h, validAccountBody(map[string]any{"application_id": "app_y"}))
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_y?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got domain.Account
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Currency != "RUB" {
		t.Errorf("currency=%q want RUB", got.Currency)
	}
}

func TestAccount_GetNotFound(t *testing.T) {
	h, _ := newAccountHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

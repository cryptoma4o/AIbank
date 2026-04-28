package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/aibank/platform/services/notification-service/internal/domain"
	"github.com/aibank/platform/services/notification-service/internal/sender"
	"github.com/aibank/platform/services/notification-service/internal/templates"
)

func newTestServer(t *testing.T) (*chi.Mux, *InMemoryRepo, *sender.MockSender) {
	t.Helper()
	tpl, err := templates.NewRegistry()
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	repo := NewInMemoryRepo()
	mock := sender.NewMockSender(domain.RecipientEmail)
	senders := map[domain.RecipientType]sender.Sender{
		domain.RecipientEmail: mock,
		domain.RecipientSMS:   sender.NewMockSender(domain.RecipientSMS),
	}
	h := New(repo, tpl, senders, nil)
	r := chi.NewMux()
	r.Mount("/v1", h.Routes())
	return r, repo, mock
}

func doJSON(t *testing.T, mux *chi.Mux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestCreate_HappyPath(t *testing.T) {
	mux, repo, mock := newTestServer(t)
	rec := doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id":      "alpha",
		"recipient_type": "email",
		"recipient":      "ivan@example.com",
		"template_id":    "applicant_welcome",
		"vars": map[string]string{
			"full_name": "Иван Иванов",
			"bank_name": "Альфа-Банк",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp domain.Notification
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != domain.StatusSent {
		t.Errorf("expected status=sent, got %s", resp.Status)
	}
	if resp.AttemptCount != 1 {
		t.Errorf("expected attempt_count=1, got %d", resp.AttemptCount)
	}

	got, err := repo.GetByID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("repo GetByID: %v", err)
	}
	if got.Status != domain.StatusSent {
		t.Errorf("repo status mismatch: %s", got.Status)
	}

	rs := mock.Records()
	if len(rs) != 1 || !strings.Contains(rs[0].Subject, "Иван Иванов") {
		t.Fatalf("mock didn't receive rendered subject: %+v", rs)
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	mux, _, _ := newTestServer(t)

	rec := doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
		"recipient_type": "email", "recipient": "x@y", "template_id": "otp",
		"vars": map[string]string{"code": "1", "ttl_minutes": "5"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing tenant_id, got %d", rec.Code)
	}

	rec = doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id": "alpha", "recipient_type": "fax", "recipient": "x",
		"template_id": "otp", "vars": map[string]string{"code": "1", "ttl_minutes": "5"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad recipient_type, got %d", rec.Code)
	}

	rec = doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id": "alpha", "recipient_type": "email", "recipient": "x@y",
		"template_id": "applicant_welcome", "vars": map[string]string{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing var, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_ChannelUnavailable(t *testing.T) {
	tpl, _ := templates.NewRegistry()
	repo := NewInMemoryRepo()
	senders := map[domain.RecipientType]sender.Sender{
		domain.RecipientEmail: sender.NewMockSender(domain.RecipientEmail),
	}
	h := New(repo, tpl, senders, nil)
	mux := chi.NewMux()
	mux.Mount("/v1", h.Routes())

	rec := doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
		"tenant_id": "alpha", "recipient_type": "push", "recipient": "tok",
		"template_id": "otp", "vars": map[string]string{"code": "1", "ttl_minutes": "5"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "channel_unavailable") {
		t.Errorf("expected channel_unavailable error: %s", rec.Body.String())
	}
}

func TestList(t *testing.T) {
	mux, _, _ := newTestServer(t)
	for i := 0; i < 2; i++ {
		_ = doJSON(t, mux, http.MethodPost, "/v1/notifications", map[string]any{
			"tenant_id": "alpha", "recipient_type": "email",
			"recipient": "x@y", "template_id": "otp",
			"vars": map[string]string{"code": "1", "ttl_minutes": "5"},
		})
	}
	rec := doJSON(t, mux, http.MethodGet, "/v1/notifications?tenant_id=alpha&limit=10", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Items []domain.Notification `json:"items"`
		Count int                   `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d/%d", resp.Count, len(resp.Items))
	}
}

func TestList_TenantRequired(t *testing.T) {
	mux, _, _ := newTestServer(t)
	rec := doJSON(t, mux, http.MethodGet, "/v1/notifications", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 missing tenant_id, got %d", rec.Code)
	}
}

func TestGet_NotFound(t *testing.T) {
	mux, _, _ := newTestServer(t)
	rec := doJSON(t, mux, http.MethodGet, "/v1/notifications/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

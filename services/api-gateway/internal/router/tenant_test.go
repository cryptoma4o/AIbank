package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolver_FromSubdomain(t *testing.T) {
	r := NewResolver([]string{"platform.ru", "localhost"})
	cases := []struct {
		host string
		want string
		ok   bool
	}{
		{"bank-alpha.platform.ru", "", false},               // hyphen — не пройдёт regex
		{"alpha.platform.ru", "alpha", true},                // ok
		{"alpha.platform.ru:8000", "alpha", true},           // с портом
		{"alfa_bank.platform.ru", "alfa_bank", true},        // ok с подчёркиванием
		{"alpha.localhost", "alpha", true},                  // dev
		{"alpha.localhost:8000", "alpha", true},             // dev с портом
		{"platform.ru", "", false},                          // голый домен
		{"foo.bar.platform.ru", "", false},                  // multi-level subdomain
		{"unknown.example.com", "", false},                  // не из suffix-list
		{"ALPHA.platform.ru", "alpha", true},                // case-insensitive
		{"1bad.platform.ru", "", false},                     // должен начинаться с буквы
		{"a.platform.ru", "", false},                        // короче 2 символов
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Host = c.host
		got, ok := r.Resolve(req)
		if got != c.want || ok != c.ok {
			t.Errorf("host=%s: got (%q,%v), want (%q,%v)", c.host, got, ok, c.want, c.ok)
		}
	}
}

func TestResolver_HeaderFallback(t *testing.T) {
	r := NewResolver(nil)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "unknown-host.invalid"
	req.Header.Set(HeaderTenantID, "alpha")
	got, ok := r.Resolve(req)
	if !ok || got != "alpha" {
		t.Fatalf("expected (alpha,true), got (%q,%v)", got, ok)
	}
}

func TestResolver_HeaderInvalidRegex(t *testing.T) {
	r := NewResolver(nil)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderTenantID, "BAD-Tenant!")
	got, ok := r.Resolve(req)
	if ok || got != "" {
		t.Fatalf("expected (\"\",false), got (%q,%v)", got, ok)
	}
}

func TestResolver_HeaderTakesPrecedence(t *testing.T) {
	r := NewResolver(nil)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "beta.platform.ru"
	req.Header.Set(HeaderTenantID, "alpha")
	got, _ := r.Resolve(req)
	if got != "alpha" {
		t.Fatalf("expected header to win, got %q", got)
	}
}

func TestMiddleware_400OnUnresolved(t *testing.T) {
	r := NewResolver(nil)
	called := false
	h := r.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding/x", nil)
	req.Host = "not-a-tenant-host.invalid"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if called {
		t.Fatal("next must not be called when tenant is unresolved")
	}
}

func TestMiddleware_PassesAndSetsContext(t *testing.T) {
	r := NewResolver([]string{"platform.ru"})
	var seenHeader, seenCtx string
	h := r.Middleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenHeader = req.Header.Get(HeaderTenantID)
		seenCtx = FromContext(req.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Host = "alpha.platform.ru"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if seenHeader != "alpha" || seenCtx != "alpha" {
		t.Fatalf("expected header=ctx=alpha, got header=%q ctx=%q", seenHeader, seenCtx)
	}
}

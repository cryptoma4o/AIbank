package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouter_RoutesByPrefix(t *testing.T) {
	onboarding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("onboarding:" + r.URL.Path))
	}))
	defer onboarding.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("admin:" + r.URL.Path))
	}))
	defer admin.Close()
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("llm:" + r.URL.Path))
	}))
	defer llm.Close()

	rt, err := NewRouter(Config{
		BFFOnboardingURL: onboarding.URL,
		BFFAdminURL:      admin.URL,
		LLMGatewayURL:    llm.URL,
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	cases := []struct {
		path string
		want string
	}{
		{"/api/onboarding/v1/applications", "onboarding:/api/onboarding/v1/applications"},
		{"/api/admin/v1/applications", "admin:/api/admin/v1/applications"},
		{"/api/llm/v1/chat/completions", "llm:/api/llm/v1/chat/completions"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("path=%s: expected 200, got %d", c.path, rec.Code)
			continue
		}
		body, _ := io.ReadAll(rec.Body)
		if string(body) != c.want {
			t.Errorf("path=%s: want %q, got %q", c.path, c.want, string(body))
		}
	}
}

func TestRouter_404OnUnknownPrefix(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer backend.Close()
	rt, err := NewRouter(Config{BFFOnboardingURL: backend.URL})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/unknown/x", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "not_found") {
		t.Fatalf("expected not_found in body, got: %s", body)
	}
}

func TestRouter_LongerPrefixWins(t *testing.T) {
	short := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("short"))
	}))
	defer short.Close()
	long := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("long"))
	}))
	defer long.Close()

	rt, err := NewRouterFromRoutes([]Route{
		{Prefix: "/api/", Target: short.URL},
		{Prefix: "/api/admin/", Target: long.URL},
	})
	if err != nil {
		t.Fatalf("NewRouterFromRoutes: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/x", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "long" {
		t.Fatalf("expected longer prefix to win, got %q", body)
	}
}

func TestRouter_502OnUpstreamDown(t *testing.T) {
	rt, err := NewRouter(Config{BFFOnboardingURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding/x", nil)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestNewRouter_RejectsInvalidTarget(t *testing.T) {
	if _, err := NewRouter(Config{BFFOnboardingURL: "not-a-url"}); err == nil {
		t.Fatal("expected error on bad URL")
	}
}

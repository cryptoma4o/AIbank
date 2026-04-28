package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aibank/api-gateway/internal/router"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "this-is-a-32-byte-long-secret-OK"

func issueTok(t *testing.T, secret []byte, tenantID, role string, ttl time.Duration) string {
	t.Helper()
	c := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "u_1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		TenantID: tenantID,
		Role:     role,
		Type:     "access",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

// TestMiddleware_RejectsMissingToken — нет Authorization → 401.
func TestMiddleware_RejectsMissingToken(t *testing.T) {
	v, err := NewVerifier([]byte(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	mw := Middleware(v, Options{Optional: false})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing_token") {
		t.Fatalf("expected missing_token, got %s", rec.Body.String())
	}
}

// TestMiddleware_OptionalAllowsMissingToken — Optional=true, нет токена → пропуск.
func TestMiddleware_OptionalAllowsMissingToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: true})
	hit := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !hit {
		t.Fatal("expected handler to be invoked when Optional=true")
	}
}

// TestMiddleware_RejectsExpired — просроченный access → 401 invalid_token.
func TestMiddleware_RejectsExpired(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), "alpha", "client.applicant", -time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Fatalf("expected invalid_token, got %s", rec.Body.String())
	}
}

// TestMiddleware_RejectsInvalidSignature — неверный secret → 401.
func TestMiddleware_RejectsInvalidSignature(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(strings.Repeat("X", 32)), "alpha", "client.applicant", time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got %d", rec.Code)
	}
}

// TestMiddleware_TenantMismatch403 — tenant claim ≠ субдомен → 403.
func TestMiddleware_TenantMismatch403(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false, EnforceTenantMatch: true})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), "alpha", "bank.admin", time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	// субдомен говорит beta — должен быть 403
	req = req.WithContext(router.WithContext(req.Context(), "beta"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "tenant_mismatch") {
		t.Fatalf("expected tenant_mismatch, got %s", rec.Body.String())
	}
}

// TestMiddleware_TenantMatchOK — валидный токен + матч → 200, headers заполнены.
func TestMiddleware_TenantMatchOK(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false, EnforceTenantMatch: true})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(HeaderActorID); got != "u_1" {
			t.Errorf("X-Actor-ID: want u_1, got %q", got)
		}
		if got := r.Header.Get(HeaderTenantID); got != "alpha" {
			t.Errorf("X-Tenant-ID: want alpha, got %q", got)
		}
		if got := r.Header.Get(HeaderActorType); got != ActorTypeUser {
			t.Errorf("X-Actor-Type: want %s, got %q", ActorTypeUser, got)
		}
		if got := r.Header.Get(HeaderAuthRole); got != "bank.admin" {
			t.Errorf("X-Auth-Role: want bank.admin, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), "alpha", "bank.admin", time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req = req.WithContext(router.WithContext(req.Context(), "alpha"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestMiddleware_StripsClientIdentityHeaders — клиентские X-Actor-ID/X-Tenant-ID
// должны быть переписаны (защита от spoofing).
func TestMiddleware_StripsClientIdentityHeaders(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false})
	var seenActor, seenTenant string
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seenActor = r.Header.Get(HeaderActorID)
		seenTenant = r.Header.Get(HeaderTenantID)
	}))
	tok := issueTok(t, []byte(testSecret), "alpha", "bank.admin", time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	// Атакующий пытается выдать себя за другого actor'а.
	req.Header.Set(HeaderActorID, "u_attacker")
	req.Header.Set(HeaderTenantID, "victim_tenant")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seenActor != "u_1" {
		t.Fatalf("X-Actor-ID was not overwritten: got %q (header injection!)", seenActor)
	}
	if seenTenant != "alpha" {
		t.Fatalf("X-Tenant-ID was not overwritten: got %q (header injection!)", seenTenant)
	}
}

// TestMiddleware_PublicRouteBypass — публичный путь не требует токена и
// гарантированно очищает identity-headers (даже если клиент прислал).
func TestMiddleware_PublicRouteBypass(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{
		Optional:     false,
		PublicRoutes: []string{"/api/onboarding/v1/auth/login", "/api/onboarding/v1/applicants"},
	})
	var seenActor string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenActor = r.Header.Get(HeaderActorID)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/v1/auth/login", nil)
	// Никакого Authorization, а вот клиент пытается прокинуть identity.
	req.Header.Set(HeaderActorID, "u_spoofed")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on public route, got %d", rec.Code)
	}
	if seenActor != "" {
		t.Fatalf("expected identity headers stripped on public route, got X-Actor-ID=%q", seenActor)
	}
}

// TestMiddleware_RejectsRefreshToken — refresh-токен не должен проходить
// edge: refresh обслуживается напрямую identity-service'ом.
func TestMiddleware_RejectsRefreshToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	mw := Middleware(v, Options{Optional: false})
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not be called for refresh token")
	}))
	c := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "u_1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: "alpha",
		Role:     "bank.admin",
		Type:     "refresh",
	}
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(testSecret))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestActorTypeFromRole — мапинг ролей в actor-type.
func TestActorTypeFromRole(t *testing.T) {
	cases := []struct {
		role string
		want string
	}{
		{"client.applicant", ActorTypeApplicant},
		{"", ActorTypeApplicant}, // applicant без явной роли
		{"bank.admin", ActorTypeUser},
		{"bank.compliance_officer", ActorTypeUser},
		{"platform.admin", ActorTypeUser},
		{"service.audit", ActorTypeService},
	}
	for _, c := range cases {
		if got := actorTypeFromRole(c.role); got != c.want {
			t.Errorf("role=%q: want %q, got %q", c.role, c.want, got)
		}
	}
}

// TestParsePublicRoutes — парсер PUBLIC_ROUTES env.
func TestParsePublicRoutes(t *testing.T) {
	got := ParsePublicRoutes(" /a , /b/c , , /d ")
	want := []string{"/a", "/b/c", "/d"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("at %d: got %q want %q", i, got[i], want[i])
		}
	}
	if ParsePublicRoutes("") != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestNewVerifier_RejectsShortSecret(t *testing.T) {
	if _, err := NewVerifier([]byte("short")); err == nil {
		t.Fatal("expected error on short secret")
	}
}

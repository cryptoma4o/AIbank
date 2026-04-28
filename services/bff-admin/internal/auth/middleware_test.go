package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "this-is-a-32-byte-long-secret-OK"

func issueTok(t *testing.T, secret []byte, role string) string {
	t.Helper()
	return issueTokTTL(t, secret, role, time.Minute)
}

func issueTokTTL(t *testing.T, secret []byte, role string, ttl time.Duration) string {
	t.Helper()
	c := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "u_admin",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		TenantID: "alpha",
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

func TestMiddleware_RejectsOperator(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	called := false
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	tok := issueTok(t, []byte(testSecret), RoleBankOperator)
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for bank.operator, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "forbidden") {
		t.Fatalf("expected forbidden in body, got %s", rec.Body.String())
	}
	if called {
		t.Fatal("handler must not be called for forbidden role")
	}
}

func TestMiddleware_AcceptsBankAdmin(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	var seen *AuthContext
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac, _ := FromContext(r.Context())
		seen = ac
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), RoleBankAdmin)
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if seen == nil || seen.UserID != "u_admin" || seen.Role != RoleBankAdmin || seen.TenantID != "alpha" {
		t.Fatalf("unexpected ctx: %+v", seen)
	}
}

func TestMiddleware_AcceptsPlatformAdmin(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), RolePlatformAdmin)
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for platform.admin, got %d", rec.Code)
	}
}

func TestMiddleware_RejectsMissing(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRoleHelpers(t *testing.T) {
	if !IsAdmin(RoleBankAdmin) || !IsAdmin(RolePlatformAdmin) || !IsAdmin(RoleBankComplianceOfficer) {
		t.Fatal("bank.admin / bank.compliance_officer / platform.admin should pass IsAdmin")
	}
	if IsAdmin(RoleBankOperator) {
		t.Fatal("operator must not pass IsAdmin")
	}
	if IsAdmin("client.applicant") {
		t.Fatal("applicant must not pass IsAdmin")
	}
	if !IsPlatformAdmin(RolePlatformAdmin) {
		t.Fatal("platform.admin should pass IsPlatformAdmin")
	}
	if IsPlatformAdmin(RoleBankAdmin) || IsPlatformAdmin(RoleBankComplianceOfficer) {
		t.Fatal("bank.* roles must not pass IsPlatformAdmin")
	}
}

// TestMiddleware_AcceptsComplianceOfficer — комплаенс-офицер должен
// проходить (см. docs/security-architecture.md § 3.2).
func TestMiddleware_AcceptsComplianceOfficer(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac, ok := FromContext(r.Context())
		if !ok || ac.Role != RoleBankComplianceOfficer {
			t.Fatalf("AuthContext missing or wrong role: %+v", ac)
		}
		w.WriteHeader(http.StatusOK)
	}))
	tok := issueTok(t, []byte(testSecret), RoleBankComplianceOfficer)
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for bank.compliance_officer, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestMiddleware_RejectsExpired — просроченный access → 401 invalid_token.
func TestMiddleware_RejectsExpired(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not be called for expired token")
	}))
	tok := issueTokTTL(t, []byte(testSecret), RoleBankAdmin, -time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Fatalf("expected invalid_token, got %s", rec.Body.String())
	}
}

// TestMiddleware_RejectsApplicant — client.applicant не имеет доступа в admin-панель.
func TestMiddleware_RejectsApplicant(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	h := Middleware(v)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not be called for applicant role")
	}))
	tok := issueTok(t, []byte(testSecret), "client.applicant")
	req := httptest.NewRequest(http.MethodPost, "/graphql/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for applicant, got %d", rec.Code)
	}
}

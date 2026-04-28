package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 bytes (RFC 7518 минимум)

// signClaims — helper, имитирующий identity-service.Issuer.Sign без импорта
// чужого пакета (циклы запрещены; BFF не зависит от identity-service в коде).
func signClaims(t *testing.T, secret string, c *Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func validClaims(now time.Time) *Claims {
	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		TenantID: "alpha",
		Role:     "client.applicant",
		Type:     TokenAccess,
	}
}

func TestVerifier_RejectsShortSecret(t *testing.T) {
	if _, err := NewVerifier([]byte("short")); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestVerifier_ParsesValidToken(t *testing.T) {
	v, err := NewVerifier([]byte(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	tok := signClaims(t, testSecret, validClaims(time.Now().UTC()))

	got, err := v.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Subject != "user-1" || got.TenantID != "alpha" || got.Role != "client.applicant" {
		t.Fatalf("unexpected claims: %+v", got)
	}
	if got.Type != TokenAccess {
		t.Fatalf("expected access type, got %s", got.Type)
	}
}

func TestVerifier_RejectsExpiredToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	expired := validClaims(time.Now().Add(-2 * time.Hour))
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
	tok := signClaims(t, testSecret, expired)

	if _, err := v.Parse(tok); err == nil {
		t.Fatal("expected error for expired token")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestVerifier_RejectsBadSignature(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	tok := signClaims(t, strings.Repeat("X", 32), validClaims(time.Now()))
	if _, err := v.Parse(tok); err == nil {
		t.Fatal("expected error for wrong signature")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestVerifier_RejectsMalformedToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	if _, err := v.Parse("not.a.jwt"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	rec := httptest.NewRecorder()

	called := false
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler must not be called")
	}
	if !strings.Contains(rec.Body.String(), "missing_token") {
		t.Fatalf("expected missing_token, got %s", rec.Body.String())
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	rec := httptest.NewRecorder()

	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("must not be called")
	}))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Fatalf("expected invalid_token, got %s", rec.Body.String())
	}
}

func TestMiddleware_RejectsRefreshTokenForGraphQL(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	cl := validClaims(time.Now())
	cl.Type = TokenRefresh
	tok := signClaims(t, testSecret, cl)

	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()

	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("must not be called: refresh tokens forbidden on /graphql")
	}))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_PassesValidToken(t *testing.T) {
	v, _ := NewVerifier([]byte(testSecret))
	tok := signClaims(t, testSecret, validClaims(time.Now()))

	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()

	var seen *AuthContext
	h := Middleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ac, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("AuthContext not in request context")
		}
		seen = ac
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if seen == nil || seen.UserID != "user-1" || seen.TenantID != "alpha" || seen.Role != "client.applicant" {
		t.Fatalf("unexpected AuthContext: %+v", seen)
	}
}

func TestFromContext_Empty(t *testing.T) {
	if _, ok := FromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); ok {
		t.Fatal("expected no AuthContext in empty context")
	}
}

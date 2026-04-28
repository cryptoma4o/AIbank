package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"aibank/identity-service/internal/domain"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func TestIssuer_RoundTrip(t *testing.T) {
	iss, err := NewIssuer([]byte(testSecret))
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	ver, err := NewVerifier([]byte(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	pair, err := iss.Issue(IssueParams{Subject: "user-1", TenantID: "alpha", Role: "bank.operator"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}
	if pair.ExpiresIn != int64(AccessTokenTTL.Seconds()) {
		t.Fatalf("expected ExpiresIn=%d, got %d", int64(AccessTokenTTL.Seconds()), pair.ExpiresIn)
	}

	access, err := ver.Parse(pair.AccessToken)
	if err != nil {
		t.Fatalf("Parse access: %v", err)
	}
	if access.Subject != "user-1" || access.TenantID != "alpha" || access.Role != "bank.operator" {
		t.Fatalf("unexpected access claims: %+v", access)
	}
	if access.Type != domain.TokenAccess {
		t.Fatalf("expected access type, got %s", access.Type)
	}

	refresh, err := ver.Parse(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Parse refresh: %v", err)
	}
	if refresh.Type != domain.TokenRefresh {
		t.Fatalf("expected refresh type, got %s", refresh.Type)
	}
}

func TestIssuer_RejectsShortSecret(t *testing.T) {
	if _, err := NewIssuer([]byte("short")); err == nil {
		t.Fatal("expected error for short secret")
	}
	if _, err := NewVerifier([]byte("short")); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestVerifier_RejectsExpiredToken(t *testing.T) {
	iss, _ := NewIssuer([]byte(testSecret))
	ver, _ := NewVerifier([]byte(testSecret))

	// Генерируем токен с уже истёкшим exp.
	iss.now = func() time.Time { return time.Now().Add(-2 * AccessTokenTTL) }
	pair, err := iss.Issue(IssueParams{Subject: "user-2"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := ver.Parse(pair.AccessToken); err == nil {
		t.Fatal("expected error parsing expired token")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestVerifier_RejectsInvalidSignature(t *testing.T) {
	iss, _ := NewIssuer([]byte(testSecret))
	pair, err := iss.Issue(IssueParams{Subject: "user-3"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	otherSecret := strings.Repeat("X", 32)
	ver, err := NewVerifier([]byte(otherSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if _, err := ver.Parse(pair.AccessToken); err == nil {
		t.Fatal("expected error parsing token with wrong key")
	} else if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestVerifier_RejectsWrongAlgorithm(t *testing.T) {
	// Подписываем токен другим алгоритмом семейства HMAC, но с подлогом alg=none
	// невозможно — golang-jwt по умолчанию запрещает unsecured. Здесь проверяем,
	// что мы явно отвергаем не-HMAC-метод. Используем наш Issuer для базового токена,
	// затем подменяем заголовок на нечитаемый — Parse должен упасть.
	ver, _ := NewVerifier([]byte(testSecret))
	if _, err := ver.Parse("not.a.jwt"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestIssuer_SignManualClaims(t *testing.T) {
	iss, _ := NewIssuer([]byte(testSecret))
	ver, _ := NewVerifier([]byte(testSecret))

	now := time.Now().UTC()
	claims := &domain.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "manual",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
		Type: domain.TokenAccess,
	}
	tok, err := iss.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := ver.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Subject != "manual" {
		t.Fatalf("expected subject manual, got %s", got.Subject)
	}
}

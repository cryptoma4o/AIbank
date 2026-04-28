// Package auth — JWT middleware для bff-onboarding.
//
// Разбирает Authorization: Bearer <token>, валидирует HS256-подпись
// общим JWT_SECRET'ом identity-service'а (см. services/identity-service/
// internal/auth/jwt.go) и кладёт AuthContext в request context.
//
// Анонимные эндпоинты (login/registration) обслуживаются identity-service
// напрямую через api-gateway, не через BFF.  В BFF любой /graphql-запрос
// обязан иметь access-токен; исключения — /health, /ready, /playground.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// tokenIssuer должен совпадать с identity-service.
const tokenIssuer = "aibank-identity"

// ErrInvalidToken — единая ошибка для всех некорректных токенов.
//
// Совпадает по namespace с identity-service, чтобы клиенты могли
// одинаково реагировать на 401.
var ErrInvalidToken = errors.New("invalid token")

// TokenType — тип токена в claims.
type TokenType string

const (
	TokenAccess  TokenType = "access"
	TokenRefresh TokenType = "refresh"
)

// Claims — точное зеркало identity-service/internal/domain.JWTClaims.
//
// При расхождении формата identity-service первичен; этот тип нужно
// синхронизировать.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string    `json:"tenant_id,omitempty"`
	Role     string    `json:"role,omitempty"`
	Type     TokenType `json:"type"`
}

// AuthContext — то, что резолверы достают через FromContext.
type AuthContext struct {
	UserID   string
	TenantID string
	Role     string
}

type ctxKey struct{}

// authCtxKey — приватный ключ контекста, чтобы исключить коллизии.
var authCtxKey = ctxKey{}

// Verifier — лёгкая обёртка над секретом.  Один экземпляр на процесс.
type Verifier struct {
	secret []byte
}

// NewVerifier требует secret >= 32 байт (RFC 7518 §3.2; то же ограничение,
// что в identity-service).
func NewVerifier(secret []byte) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Verifier{secret: secret}, nil
}

// Parse валидирует подпись/срок и возвращает claims.
func (v *Verifier) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Middleware возвращает chi/net-http совместимый middleware.
//
// Поведение:
//   - нет Authorization header → 401 missing_token
//   - не Bearer → 401 missing_token
//   - не access token (refresh) → 401 invalid_token
//   - подпись/срок не сошлись → 401 invalid_token
//   - claims OK → AuthContext в context.Context, next.ServeHTTP
func Middleware(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := bearerToken(r)
			if !ok {
				writeAuthError(w, http.StatusUnauthorized, "missing_token",
					"Authorization: Bearer <token> required")
				return
			}
			claims, err := v.Parse(tok)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token",
					"token is invalid or expired")
				return
			}
			if claims.Type != TokenAccess {
				writeAuthError(w, http.StatusUnauthorized, "invalid_token",
					"expected an access token")
				return
			}
			ac := &AuthContext{
				UserID:   claims.Subject,
				TenantID: claims.TenantID,
				Role:     claims.Role,
			}
			ctx := context.WithValue(r.Context(), authCtxKey, ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext возвращает AuthContext из контекста; bool=false если нет.
func FromContext(ctx context.Context) (*AuthContext, bool) {
	ac, ok := ctx.Value(authCtxKey).(*AuthContext)
	return ac, ok
}

// WithContext — для тестов: возвращает контекст с AuthContext внутри.
func WithContext(ctx context.Context, ac *AuthContext) context.Context {
	return context.WithValue(ctx, authCtxKey, ac)
}

// bearerToken — извлекает <token> из "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	t := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if t == "" {
		return "", false
	}
	return t, true
}

// writeAuthError — единый формат 401 в стиле identity-service.
func writeAuthError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, code, msg)
	_, _ = w.Write([]byte(body))
}

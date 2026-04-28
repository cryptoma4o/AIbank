// Package auth — JWT-middleware и role-check для bff-admin.
//
// Принцип: bff-admin доступен только сотрудникам банка.  Роль проверяется
// здесь — каждый GraphQL-запрос требует роли `bank.admin` или
// `platform.admin`.  Тонкая авторизация на уровне отдельных мутаций
// (suspendTenant требует platform.admin) — в резолверах.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const tokenIssuer = "aibank-identity"

// Роли, известные платформе (синхронизировано с identity-service).
//
// Допуск в bff-admin (см. docs/security-architecture.md § 3.2):
//   - bank.admin             — полные admin-права в рамках тенанта
//   - bank.compliance_officer — комплаенс-офицер: read-only + решения
//   - platform.admin          — наш админ (cross-tenant ops)
//
// bank.operator оператор первой линии не имеет доступа в admin-панель —
// у него отдельный bff-operator (TODO).
const (
	RoleBankAdmin             = "bank.admin"
	RoleBankComplianceOfficer = "bank.compliance_officer"
	RoleBankOperator          = "bank.operator"
	RolePlatformAdmin         = "platform.admin"
)

// ErrInvalidToken — некорректный JWT.
var ErrInvalidToken = errors.New("invalid token")

// ErrForbidden — токен валиден, но роль не подходит.
var ErrForbidden = errors.New("forbidden")

// Claims — зеркалит identity-service JWTClaims.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenant_id,omitempty"`
	Role     string `json:"role,omitempty"`
	Type     string `json:"type"`
}

// AuthContext — то, что резолверы достают через FromContext.
type AuthContext struct {
	UserID   string
	TenantID string
	Role     string
}

type ctxKey struct{}

var authCtxKey = ctxKey{}

// Verifier — обёртка над JWT_SECRET.
type Verifier struct{ secret []byte }

func NewVerifier(secret []byte) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Verifier{secret: secret}, nil
}

// Parse валидирует подпись/срок и возвращает claims.
func (v *Verifier) Parse(tokenStr string) (*Claims, error) {
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (any, error) {
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
	return c, nil
}

// IsAdmin — true если роль удовлетворяет требованиям admin-доступа.
//
// Allowlist: bank.admin, bank.compliance_officer, platform.admin.
// Все остальные (включая bank.operator) → 403.
func IsAdmin(role string) bool {
	return role == RoleBankAdmin ||
		role == RoleBankComplianceOfficer ||
		role == RolePlatformAdmin
}

// IsPlatformAdmin — true только для platform.admin (cross-tenant ops).
func IsPlatformAdmin(role string) bool { return role == RolePlatformAdmin }

// Middleware — JWT validate + role check.
//
//   - 401 missing_token: нет Authorization
//   - 401 invalid_token: подпись/срок
//   - 403 forbidden: роль не bank.admin / platform.admin
func Middleware(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := bearer(r)
			if !ok {
				writeAuthErr(w, http.StatusUnauthorized, "missing_token",
					"Authorization: Bearer <token> required")
				return
			}
			c, err := v.Parse(tok)
			if err != nil {
				writeAuthErr(w, http.StatusUnauthorized, "invalid_token",
					"token is invalid or expired")
				return
			}
			if !IsAdmin(c.Role) {
				writeAuthErr(w, http.StatusForbidden, "forbidden",
					"admin role required (bank.admin or platform.admin)")
				return
			}
			ac := &AuthContext{UserID: c.Subject, TenantID: c.TenantID, Role: c.Role}
			ctx := context.WithValue(r.Context(), authCtxKey, ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext возвращает AuthContext или (nil,false).
func FromContext(ctx context.Context) (*AuthContext, bool) {
	ac, ok := ctx.Value(authCtxKey).(*AuthContext)
	return ac, ok
}

// WithContext — хелпер для тестов.
func WithContext(ctx context.Context, ac *AuthContext) context.Context {
	return context.WithValue(ctx, authCtxKey, ac)
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return "", false
	}
	t := strings.TrimSpace(strings.TrimPrefix(h, p))
	return t, t != ""
}

func writeAuthErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"error":{"code":"` + code + `","message":"` + msg + `"}}`
	_, _ = w.Write([]byte(body))
}

// Package auth — JWT-валидация и установка identity-headers в api-gateway.
//
// Назначение этого слоя — единственная точка проверки подписи токена на
// edge.  После него downstream-сервисы (bff-onboarding, bff-admin,
// onboarding-orchestrator и т.д.) получают доверенные identity-headers:
//
//	X-Tenant-ID    — tenant из claim'а (или пусто для платформенных ролей)
//	X-Actor-ID     — sub из claim'а (user.id или applicant.id)
//	X-Actor-Type   — "user" | "applicant" | "service" — производное от роли
//	X-Auth-Role    — роль из claim'а (для аудита/логов)
//	X-Auth-Subject — алиас X-Actor-ID (legacy для совместимости)
//
// Эти заголовки ВСЕГДА переписываются gateway'ем — клиентские значения
// игнорируются, чтобы исключить header-injection (CVE-class A01).
//
// Public routes:
// Часть эндпоинтов (login, регистрация applicant'а) обслуживается без
// токена.  Они описываются env PUBLIC_ROUTES (csv префиксов URL); для
// них middleware пропускает запрос, очищая identity-headers, чтобы
// downstream не получили подделанных значений.
//
// Секрет — общий с identity-service (см. JWT_SECRET, RFC 7518 §3.2).
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"aibank/api-gateway/internal/router"

	"github.com/golang-jwt/jwt/v5"
)

const tokenIssuer = "aibank-identity"

// Identity-headers, которые gateway записывает downstream.  Перечислены
// здесь, чтобы тесты и downstream-сервисы могли использовать одни и те
// же константы и не дрейфовать.
const (
	HeaderTenantID  = "X-Tenant-ID"
	HeaderActorID   = "X-Actor-ID"
	HeaderActorType = "X-Actor-Type"
	HeaderAuthRole  = "X-Auth-Role"
	HeaderAuthSub   = "X-Auth-Subject"
)

// ActorType — производное от роли значение для X-Actor-Type.
const (
	ActorTypeUser      = "user"      // bank.* / platform.*
	ActorTypeApplicant = "applicant" // client.applicant
	ActorTypeService   = "service"   // service.* (зарезервировано под service JWT)
)

// ErrUnauthorized — единый sentinel для некорректных/отсутствующих
// токенов; downstream могут использовать errors.Is для классификации.
var ErrUnauthorized = errors.New("unauthorized")

// ErrInvalidToken — кривая подпись/срок/формат.  ErrUnauthorized
// поглощает его (см. Is).
var ErrInvalidToken = errors.New("invalid token")

// Claims — минимальный набор для edge-валидации (зеркало identity-service).
type Claims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenant_id,omitempty"`
	Role     string `json:"role,omitempty"`
	Type     string `json:"type"`
}

// Verifier — обёртка над общим JWT_SECRET.
type Verifier struct{ secret []byte }

// NewVerifier — secret >= 32 байт (RFC 7518 §3.2).
func NewVerifier(secret []byte) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Verifier{secret: secret}, nil
}

// Parse — валидирует подпись и срок.
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

// Options — настройки middleware'а.
type Options struct {
	// Optional=true → отсутствие Authorization не приводит к 401 (legacy
	// dev-режим, когда JWT_SECRET ещё не настроен).  При наличии токена
	// он всё равно валидируется.  В production используется только в
	// сочетании с PublicRoutes (см. ниже) для точечного отключения auth
	// на конкретных префиксах.
	Optional bool

	// EnforceTenantMatch=true → claim tenant_id обязан совпадать с
	// tenantID из субдомена; иначе 403 tenant_mismatch.
	EnforceTenantMatch bool

	// PublicRoutes — список URL-префиксов, для которых auth-валидация
	// пропускается.  Используется для login/registration endpoints, где
	// клиент ещё не имеет токена.  Identity-headers для таких запросов
	// гарантированно очищаются (защита от header-injection).
	//
	// Пример: ["/api/onboarding/v1/auth/login", "/api/onboarding/v1/applicants"].
	PublicRoutes []string
}

// IsPublic — true если путь подпадает под public-route allowlist.
func (o Options) IsPublic(path string) bool {
	for _, p := range o.PublicRoutes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// Middleware — JWT validation + identity-header injection.
//
//   - public route → headers очищаются, next.ServeHTTP
//   - 401 missing_token: optional=false и нет Authorization
//   - 401 invalid_token: подпись/срок/refresh-токен на access-эндпоинте
//   - 403 tenant_mismatch: tenant в claim != tenant из субдомена
//   - на 200 пути: ставим X-Tenant-ID, X-Actor-ID, X-Actor-Type, X-Auth-*
func Middleware(v *Verifier, opts Options) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Public-route bypass: ВАЖНО очистить identity-headers, чтобы
			// исключить spoofing через клиентские заголовки.
			if opts.IsPublic(r.URL.Path) {
				stripIdentityHeaders(r)
				next.ServeHTTP(w, r)
				return
			}

			tok, ok := bearer(r)
			if !ok {
				if opts.Optional {
					stripIdentityHeaders(r)
					next.ServeHTTP(w, r)
					return
				}
				writeAuthErr(w, http.StatusUnauthorized, "missing_token",
					"Authorization: Bearer <token> required")
				return
			}
			claims, err := v.Parse(tok)
			if err != nil {
				writeAuthErr(w, http.StatusUnauthorized, "invalid_token",
					"token is invalid or expired")
				return
			}
			// Refresh-токен на edge не принимается: refresh ходит только в
			// identity-service /v1/auth/refresh, и тот сам валидирует тип.
			if claims.Type == "refresh" {
				writeAuthErr(w, http.StatusUnauthorized, "invalid_token",
					"expected an access token")
				return
			}
			if opts.EnforceTenantMatch {
				resolved := router.FromContext(r.Context())
				if resolved == "" {
					resolved = r.Header.Get(router.HeaderTenantID)
				}
				if claims.TenantID != "" && resolved != "" && claims.TenantID != resolved {
					writeAuthErr(w, http.StatusForbidden, "tenant_mismatch",
						"jwt tenant claim does not match host-derived tenant")
					return
				}
			}

			// Перезаписываем identity-headers values из claim'ов; всё, что
			// прислал клиент, игнорируется.
			stripIdentityHeaders(r)
			r.Header.Set(HeaderTenantID, claims.TenantID)
			r.Header.Set(HeaderActorID, claims.Subject)
			r.Header.Set(HeaderActorType, actorTypeFromRole(claims.Role))
			r.Header.Set(HeaderAuthRole, claims.Role)
			r.Header.Set(HeaderAuthSub, claims.Subject)
			next.ServeHTTP(w, r)
		})
	}
}

// stripIdentityHeaders — снимает все identity-headers с запроса.
//
// Защита от header-injection: даже если клиент прислал X-Actor-ID,
// downstream получит только то, что мы установим явно.
func stripIdentityHeaders(r *http.Request) {
	r.Header.Del(HeaderTenantID)
	r.Header.Del(HeaderActorID)
	r.Header.Del(HeaderActorType)
	r.Header.Del(HeaderAuthRole)
	r.Header.Del(HeaderAuthSub)
}

// actorTypeFromRole — отображение role → ActorType.
//
// Точное соответствие — см. docs/security-architecture.md § 3.2.
func actorTypeFromRole(role string) string {
	switch {
	case role == "" || role == "client.applicant":
		return ActorTypeApplicant
	case strings.HasPrefix(role, "service."):
		return ActorTypeService
	default:
		// bank.*, platform.* и любые иные операторские роли
		return ActorTypeUser
	}
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
	if t == "" {
		return "", false
	}
	return t, true
}

func writeAuthErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"error":{"code":"` + code + `","message":"` + msg + `"}}`
	_, _ = w.Write([]byte(body))
}

// ParsePublicRoutes — парсит csv-список префиксов в []string, отсекая
// пустые элементы и ведущие/хвостовые пробелы.  Используется в main.go
// для разбора env PUBLIC_ROUTES.
func ParsePublicRoutes(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

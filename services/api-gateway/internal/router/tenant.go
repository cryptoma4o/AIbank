// Package router — tenant resolution для api-gateway.
//
// Извлекает tenant_id из субдомена (`bank-alpha.platform.ru` → `bank-alpha`)
// или из header `X-Tenant-Id` (для тестов и internal-вызовов).
//
// Соглашения по идентификаторам тенантов синхронизированы с tenant-service
// (services/tenant-service/internal/repository/postgres.go) — regex
// `^[a-z][a-z0-9_]{1,31}$`.
package router

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

// TenantIDRegex — единый whitelist для tenant_id, совпадает с tenant-service.
var TenantIDRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// HeaderTenantID — fallback header для тестов и internal traffic.
const HeaderTenantID = "X-Tenant-Id"

type ctxKey struct{}

var tenantCtxKey = ctxKey{}

// TenantSuffixes — список валидных суффиксов платформы; всё, что не
// совпадает, не рассматривается как hostname платформы.
//
// Конфигурируется через ENV PLATFORM_DOMAINS (csv).  Дефолт покрывает prod
// (`platform.ru`) и dev (`localhost`).
var DefaultSuffixes = []string{"platform.ru", "localhost"}

// Resolver выделяет tenant_id из запроса.
type Resolver struct {
	suffixes []string
}

// NewResolver — конструктор; suffixes — список доменов платформы без
// ведущей точки (e.g. ["platform.ru","localhost"]).
func NewResolver(suffixes []string) *Resolver {
	if len(suffixes) == 0 {
		suffixes = DefaultSuffixes
	}
	out := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		out = append(out, strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), ".")))
	}
	return &Resolver{suffixes: out}
}

// Resolve возвращает tenant_id или ("", false) если не извлечь.
//
// Алгоритм:
//  1. Если есть X-Tenant-Id — берём его (для тестов / internal-вызовов).
//  2. Иначе пытаемся извлечь из Host: <tenant>.<suffix>.
//  3. Полученное значение валидируется TenantIDRegex.
func (r *Resolver) Resolve(req *http.Request) (string, bool) {
	if h := req.Header.Get(HeaderTenantID); h != "" {
		h = strings.ToLower(strings.TrimSpace(h))
		if TenantIDRegex.MatchString(h) {
			return h, true
		}
		return "", false
	}
	host := req.Host
	if host == "" {
		return "", false
	}
	// strip port
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.ToLower(host)

	for _, suf := range r.suffixes {
		if host == suf {
			return "", false
		}
		if strings.HasSuffix(host, "."+suf) {
			sub := strings.TrimSuffix(host, "."+suf)
			// допускаем только один уровень субдомена
			if strings.Contains(sub, ".") {
				return "", false
			}
			if TenantIDRegex.MatchString(sub) {
				return sub, true
			}
			return "", false
		}
	}
	return "", false
}

// Middleware кладёт tenant_id в context и в X-Tenant-Id для downstream.
//
// Возвращает 400 Bad Request если tenant не извлечён или не прошёл regex.
func (r *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tenantID, ok := r.Resolve(req)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "tenant_unresolved",
				"tenant not identified from Host or X-Tenant-Id")
			return
		}
		req.Header.Set(HeaderTenantID, tenantID)
		ctx := context.WithValue(req.Context(), tenantCtxKey, tenantID)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// FromContext возвращает tenant_id из контекста (или "" если нет).
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantCtxKey).(string); ok {
		return v
	}
	return ""
}

// WithContext — для тестов и internal-вызовов.
func WithContext(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey, tenantID)
}

// writeJSONError — единый формат ошибок (совпадает с identity-service).
func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"error":{"code":"` + code + `","message":"` + msg + `"}}`
	_, _ = w.Write([]byte(body))
}

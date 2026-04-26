package middleware

import (
	"net/http"
	"strings"
)

const TenantIDKey = "tenant_id"

// TenantFromSubdomain extracts tenant ID from the Host header subdomain.
// e.g. "alpha-bank.api.aibank.ru" → "alpha-bank"
// Falls back to X-Tenant-ID header for local/internal calls.
func TenantFromSubdomain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			host := r.Host
			// strip port if present
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}
			parts := strings.SplitN(host, ".", 2)
			if len(parts) >= 2 {
				tenantID = parts[0]
			}
		}
		if tenantID == "" {
			http.Error(w, `{"error":"tenant not identified"}`, http.StatusBadRequest)
			return
		}
		r.Header.Set("X-Tenant-ID", tenantID)
		next.ServeHTTP(w, r)
	})
}

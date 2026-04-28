// Package audit — простая запись доступов в slog для аудита.
//
// В будущем — отдельный pipeline в audit-service (append-only с подписью);
// здесь — минимально-полезная локальная запись для observability.
package audit

import (
	"log/slog"
	"net/http"
	"time"

	"aibank/api-gateway/internal/router"
)

// AccessLog — chi-совместимый middleware, пишет JSON-строку с метаданными
// каждого запроса.  Должен стоять ПОСЛЕ TenantResolver, чтобы tenant_id
// уже был в контексте.
func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("access",
				"tenant", router.FromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"actor", r.Header.Get("X-Auth-Subject"),
				"role", r.Header.Get("X-Auth-Role"),
			)
		})
	}
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(s int) {
	r.status = s
	r.ResponseWriter.WriteHeader(s)
}

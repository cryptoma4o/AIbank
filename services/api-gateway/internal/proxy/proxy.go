// Package proxy — простая reverse-proxy с маршрутизацией по URL-префиксу.
//
// Поддерживаемые маршруты (см. docs/technical-structure.md § 4.1):
//
//	/api/onboarding/* → bff-onboarding
//	/api/admin/*      → bff-admin
//	/api/llm/*        → llm-gateway
//	/api/audit/*      → audit-service (admin only)
//
// Префикс не отрезается: bff-onboarding ожидает /api/onboarding/...
package proxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
)

// Config — карта префиксов на upstream-URL'ы.
type Config struct {
	BFFOnboardingURL string
	BFFAdminURL      string
	LLMGatewayURL    string
	AuditServiceURL  string
}

// Route — одна запись маршрутизации.
type Route struct {
	Prefix string
	Target string
}

// ErrNoRoute — путь не подошёл ни под один префикс.
var ErrNoRoute = errors.New("proxy: no route matches")

// Router — построенный из Config реверс-прокси.
type Router struct {
	routes []routeProxy
}

type routeProxy struct {
	prefix string
	proxy  *httputil.ReverseProxy
	host   string
}

// NewRouter строит Router из Config.
func NewRouter(cfg Config) (*Router, error) {
	rts := []Route{}
	if cfg.BFFOnboardingURL != "" {
		rts = append(rts, Route{Prefix: "/api/onboarding/", Target: cfg.BFFOnboardingURL})
	}
	if cfg.BFFAdminURL != "" {
		rts = append(rts, Route{Prefix: "/api/admin/", Target: cfg.BFFAdminURL})
	}
	if cfg.LLMGatewayURL != "" {
		rts = append(rts, Route{Prefix: "/api/llm/", Target: cfg.LLMGatewayURL})
	}
	if cfg.AuditServiceURL != "" {
		rts = append(rts, Route{Prefix: "/api/audit/", Target: cfg.AuditServiceURL})
	}
	return NewRouterFromRoutes(rts)
}

// NewRouterFromRoutes — для тестов: явный список Routes.
//
// Префиксы сортируются по убыванию длины, чтобы более конкретный матч
// побеждал общий.
func NewRouterFromRoutes(rts []Route) (*Router, error) {
	out := make([]routeProxy, 0, len(rts))
	for _, r := range rts {
		u, err := url.Parse(r.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", r.Target, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid target %q: scheme and host required", r.Target)
		}
		p := httputil.NewSingleHostReverseProxy(u)
		// гасим body 502 от ReverseProxy чтобы не светить внутренний URL
		p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":"upstream_unreachable","message":"upstream is not available"}}`))
		}
		out = append(out, routeProxy{prefix: r.Prefix, proxy: p, host: u.Host})
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].prefix) > len(out[j].prefix) })
	return &Router{routes: out}, nil
}

// Match возвращает proxy и true если path подошёл; иначе nil/false.
func (r *Router) Match(path string) (*httputil.ReverseProxy, bool) {
	for _, rp := range r.routes {
		if strings.HasPrefix(path, rp.prefix) {
			return rp.proxy, true
		}
	}
	return nil, false
}

// ServeHTTP — реализует http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	p, ok := r.Match(req.URL.Path)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found",
			fmt.Sprintf("no upstream route for path %s", req.URL.Path))
		return
	}
	p.ServeHTTP(w, req)
}

// Targets — для тестов и /healthz: список (prefix → host).
func (r *Router) Targets() map[string]string {
	out := make(map[string]string, len(r.routes))
	for _, rp := range r.routes {
		out[rp.prefix] = rp.host
	}
	return out
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := `{"error":{"code":"` + code + `","message":"` + msg + `"}}`
	_, _ = w.Write([]byte(body))
}

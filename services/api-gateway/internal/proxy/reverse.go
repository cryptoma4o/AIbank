package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// DefaultRoutes maps path prefixes to backend service URLs.
var DefaultRoutes = map[string]string{
	"/v1/tenants":       "http://tenant-service:8080",
	"/v1/applications":  "http://onboarding-orchestrator:8084",
	"/v1/score":         "http://risk-engine:8085",
	"/v1/auth":          "http://identity-service:8086",
	"/v1/clients":       "http://client-service:8087",
	"/v1/graphs":        "http://ubo-service:8088",
	"/v1/documents":     "http://document-service:8082",
	"/v1/events":        "http://audit-service:8081",
	"/v1/notifications": "http://notification-service:8083",
}

type Router struct {
	routes map[string]*httputil.ReverseProxy
}

func NewRouter(routes map[string]string) (*Router, error) {
	r := &Router{routes: make(map[string]*httputil.ReverseProxy)}
	for prefix, target := range routes {
		u, err := url.Parse(target)
		if err != nil {
			return nil, fmt.Errorf("proxy: invalid target %q: %w", target, err)
		}
		r.routes[prefix] = httputil.NewSingleHostReverseProxy(u)
	}
	return r, nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for prefix, proxy := range r.routes {
		if strings.HasPrefix(req.URL.Path, prefix) {
			proxy.ServeHTTP(w, req)
			return
		}
	}
	http.Error(w, `{"error":"no route found"}`, http.StatusNotFound)
}

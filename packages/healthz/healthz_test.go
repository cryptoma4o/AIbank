package healthz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegister_AllHealthy_200(t *testing.T) {
	h := New("test-svc", "1.0.0")
	h.Register("ok1", AlwaysHealthy(""))
	h.Register("ok2", AlwaysHealthy(""))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	h.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok", resp.Status)
	}
	if len(resp.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(resp.Checks))
	}
}

func TestRegister_OneUnhealthy_503(t *testing.T) {
	h := New("test-svc", "1.0.0")
	h.Register("ok", AlwaysHealthy(""))
	h.Register("bad", FuncCheck(func(context.Context) error {
		return errors.New("boom")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	h.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "error" {
		t.Fatalf("status = %q, want error", resp.Status)
	}
	if resp.Checks["bad"].Status != StatusUnhealthy {
		t.Fatalf("bad check status = %q", resp.Checks["bad"].Status)
	}
	if !strings.Contains(resp.Checks["bad"].Message, "boom") {
		t.Fatalf("bad message = %q, want substring 'boom'", resp.Checks["bad"].Message)
	}
}

func TestDBCheck_NilDB_Unhealthy(t *testing.T) {
	check := DBCheck(nil)
	status, msg := check(context.Background())
	if status != StatusUnhealthy {
		t.Fatalf("status = %q, want unhealthy", status)
	}
	if !strings.Contains(msg, "nil") {
		t.Fatalf("message = %q, want substring 'nil'", msg)
	}
}

func TestHTTPCheck_TimeoutUnhealthy(t *testing.T) {
	// Сервер, висящий в Sleep дольше клиентского таймаута.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	check := HTTPCheck(srv.URL, 50*time.Millisecond)
	status, msg := check(context.Background())
	if status != StatusUnhealthy {
		t.Fatalf("status = %q, want unhealthy (got msg=%q)", status, msg)
	}
}

func TestJSON_StructureMatches(t *testing.T) {
	h := New("svc-x", "v2.3.4")
	h.Register("alpha", AlwaysHealthy("ok-msg"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	h.HTTPHandler().ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"status", "service", "version", "timestamp", "checks"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing key %q in JSON: %s", key, rec.Body.String())
		}
	}
	if raw["service"] != "svc-x" {
		t.Fatalf("service = %v, want svc-x", raw["service"])
	}
	if raw["version"] != "v2.3.4" {
		t.Fatalf("version = %v, want v2.3.4", raw["version"])
	}
	checks, ok := raw["checks"].(map[string]any)
	if !ok {
		t.Fatalf("checks not an object: %T", raw["checks"])
	}
	alpha, ok := checks["alpha"].(map[string]any)
	if !ok {
		t.Fatalf("checks.alpha not an object: %T", checks["alpha"])
	}
	if alpha["status"] != string(StatusHealthy) {
		t.Fatalf("alpha.status = %v", alpha["status"])
	}
}

func TestEmptyChecker_StillReturns200(t *testing.T) {
	h := New("liveness-only", "")

	// /ready без проверок: формально это «liveness-style»: 200 + ok, без checks.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	h.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want ok", resp.Status)
	}

	// Liveness handler — всегда 200.
	rec2 := httptest.NewRecorder()
	h.LivenessHandler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want 200", rec2.Code)
	}
}

func TestSoftCheck_Degraded_200(t *testing.T) {
	// Дополнительный кейс: soft-проверка падает → status="degraded", HTTP 200.
	h := New("svc", "")
	h.Register("core", AlwaysHealthy(""))
	h.Register("kafka", FuncCheck(func(context.Context) error {
		return errors.New("kafka down")
	}), Soft())

	rec := httptest.NewRecorder()
	h.HTTPHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded is still 200)", rec.Code)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", resp.Status)
	}
}

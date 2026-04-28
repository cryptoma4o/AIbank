package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"aibank/ext-spark/internal/domain"
)

type memCache struct {
	mu   sync.Mutex
	m    map[string]domain.SparkIntel
	hits int
}

func newMemCache() *memCache { return &memCache{m: make(map[string]domain.SparkIntel)} }

func (mc *memCache) Get(_ context.Context, inn string) (*domain.SparkIntel, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if v, ok := mc.m[inn]; ok {
		mc.hits++
		return &v, nil
	}
	return nil, nil
}

func (mc *memCache) Set(_ context.Context, intel *domain.SparkIntel) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.m[intel.INN] = *intel
	return nil
}

// --- tests ---

func TestIntel_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/7707083893", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", body["source"])
	}
	data := body["data"].(map[string]interface{})
	if data["inn"] != "7707083893" {
		t.Fatalf("inn mismatch: %v", data["inn"])
	}
	health := data["financial_health"].(string)
	if health != "green" && health != "yellow" && health != "red" {
		t.Fatalf("unexpected financial_health: %v", health)
	}
	news := data["news_summary"].([]interface{})
	if len(news) < 2 || len(news) > 4 {
		t.Fatalf("news_summary length out of range: %d", len(news))
	}
}

func TestIntel_CacheHit(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	req := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/7707083893", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	req2 := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/7707083893", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	var body2 map[string]interface{}
	_ = json.Unmarshal(rec2.Body.Bytes(), &body2)
	if body2["source"] != "cache" {
		t.Fatalf("expected cache, got %v", body2["source"])
	}
	if mc.hits != 1 {
		t.Fatalf("expected 1 hit, got %d", mc.hits)
	}
}

func TestIntel_DistributionRoughly(t *testing.T) {
	// Проверяем, что распределение green/yellow/red ~ 60/30/10 на 1000 ИНН.
	mc := newMemCache()
	h := NewHandler(mc)

	counts := map[string]int{"green": 0, "yellow": 0, "red": 0}
	for i := 0; i < 1000; i++ {
		inn := fmt.Sprintf("7700000%03d", i)
		req := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/"+inn, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		data := body["data"].(map[string]interface{})
		counts[data["financial_health"].(string)]++
	}
	// Tolerances: green ≥ 50%, red ≤ 20%
	if counts["green"] < 500 {
		t.Errorf("green too rare: %d/1000", counts["green"])
	}
	if counts["red"] > 200 {
		t.Errorf("red too frequent: %d/1000", counts["red"])
	}
}

func TestIntel_InvalidINN(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	cases := []string{"123", "abcdefghij", "12345"}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/"+c, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("input=%q expected 400, got %d", c, rec.Code)
		}
	}
}

func TestIntel_Determinism(t *testing.T) {
	mc1 := newMemCache()
	h1 := NewHandler(mc1)
	mc2 := newMemCache()
	h2 := NewHandler(mc2)

	req1 := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/7707083893", nil)
	rec1 := httptest.NewRecorder()
	h1.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/v1/spark/intel/7707083893", nil)
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, req2)

	if rec1.Body.String() != rec2.Body.String() {
		t.Fatalf("non-deterministic intel:\n%s\nvs\n%s", rec1.Body.String(), rec2.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	h := NewHandler(newMemCache())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

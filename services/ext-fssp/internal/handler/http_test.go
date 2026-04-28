package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"aibank/ext-fssp/internal/domain"
)

// memCache — реализация cache.Cache для тестов.
type memCache struct {
	mu     sync.Mutex
	byINN  map[string]domain.ProceedingsResult
	byPers map[string]domain.ProceedingsResult
	hits   int
}

func newMemCache() *memCache {
	return &memCache{
		byINN:  make(map[string]domain.ProceedingsResult),
		byPers: make(map[string]domain.ProceedingsResult),
	}
}

func personKey(fn, bd string) string { return fn + "|" + bd }

func (m *memCache) GetByINN(_ context.Context, inn string) (*domain.ProceedingsResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.byINN[inn]; ok {
		m.hits++
		return &v, nil
	}
	return nil, nil
}

func (m *memCache) SetByINN(_ context.Context, inn string, res *domain.ProceedingsResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byINN[inn] = *res
	return nil
}

func (m *memCache) GetByPerson(_ context.Context, fn, bd string) (*domain.ProceedingsResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.byPers[personKey(fn, bd)]; ok {
		m.hits++
		return &v, nil
	}
	return nil, nil
}

func (m *memCache) SetByPerson(_ context.Context, fn, bd string, res *domain.ProceedingsResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byPers[personKey(fn, bd)] = *res
	return nil
}

// --- helpers ---

func doGET(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.Len() == 0 {
		return rec, nil
	}
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

// --- tests ---

func TestByINN_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	rec, body := doGET(t, h, "/v1/fssp/by-inn/7707083893")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", body["source"])
	}
	data := body["data"].(map[string]interface{})
	count := int(data["count"].(float64))
	if count < 0 || count > 5 {
		t.Fatalf("count out of range: %d", count)
	}

	// Cache hit on retry
	_, body2 := doGET(t, h, "/v1/fssp/by-inn/7707083893")
	if body2["source"] != "cache" {
		t.Fatalf("expected cache, got %v", body2["source"])
	}
}

func TestByINN_FindsAtLeastOneINNWithProceedings(t *testing.T) {
	// ~5% от 200 INN ≈ 10 → должно найтись.
	mc := newMemCache()
	h := NewHandler(mc)
	totalCases := 0
	totalDebt := int64(0)
	for i := 0; i < 200; i++ {
		inn := fmt.Sprintf("770000%04d", i)
		_, body := doGET(t, h, "/v1/fssp/by-inn/"+inn)
		data := body["data"].(map[string]interface{})
		count := int(data["count"].(float64))
		if count > 0 {
			totalCases++
			totalDebt += int64(data["total_debt_kopecks"].(float64))
			// Verify items length matches count
			items := data["items"].([]interface{})
			if len(items) != count {
				t.Fatalf("count mismatch: %d vs %d", count, len(items))
			}
		}
	}
	if totalCases == 0 {
		t.Fatal("expected ≥1 INN with proceedings out of 200")
	}
	if totalCases > 30 {
		t.Fatalf("too many proceedings (%d/200) — expected ~5%%", totalCases)
	}
}

func TestByINN_InvalidFormat(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	cases := []string{"123", "abcdefghij", "12345"}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/fssp/by-inn/"+c, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("input=%q expected 400, got %d", c, rec.Code)
		}
	}
}

func TestByPerson_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	q := url.Values{}
	q.Set("full_name", "Иванов Иван Иванович")
	q.Set("birth_date", "1985-03-15")
	rec, body := doGET(t, h, "/v1/fssp/by-person?"+q.Encode())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", body["source"])
	}

	// Cache hit
	_, body2 := doGET(t, h, "/v1/fssp/by-person?"+q.Encode())
	if body2["source"] != "cache" {
		t.Fatalf("expected cache, got %v", body2["source"])
	}
}

func TestByPerson_InvalidParams(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	cases := []url.Values{
		{},
		{"full_name": []string{""}, "birth_date": []string{"1985-03-15"}},
		{"full_name": []string{"Иванов И.И."}, "birth_date": []string{""}},
		{"full_name": []string{"Иванов И.И."}, "birth_date": []string{"1985-13-45"}}, // wrong format
		{"full_name": []string{"Иванов И.И."}, "birth_date": []string{"15.03.1985"}},
	}
	for i, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/fssp/by-person?"+c.Encode(), nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d (%v): expected 400, got %d", i, c, rec.Code)
		}
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

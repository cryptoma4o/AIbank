package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"aibank/ext-egrul/internal/domain"
)

// memCache — in-memory реализация cache.Cache для тестов.
type memCache struct {
	mu       sync.Mutex
	byINN    map[string]domain.LegalEntity
	byOGRN   map[string]domain.LegalEntity
	founders map[string][]domain.Founder
	hits     int // сколько раз отдали из кэша
}

func newMemCache() *memCache {
	return &memCache{
		byINN:    make(map[string]domain.LegalEntity),
		byOGRN:   make(map[string]domain.LegalEntity),
		founders: make(map[string][]domain.Founder),
	}
}

func (m *memCache) GetByINN(_ context.Context, inn string) (*domain.LegalEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.byINN[inn]; ok {
		m.hits++
		return &v, nil
	}
	return nil, nil
}

func (m *memCache) SetByINN(_ context.Context, rec *domain.LegalEntity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byINN[rec.INN] = *rec
	if rec.OGRN != "" {
		m.byOGRN[rec.OGRN] = *rec
	}
	return nil
}

func (m *memCache) GetByOGRN(_ context.Context, ogrn string) (*domain.LegalEntity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.byOGRN[ogrn]; ok {
		m.hits++
		return &v, nil
	}
	return nil, nil
}

func (m *memCache) SetByOGRN(_ context.Context, rec *domain.LegalEntity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byOGRN[rec.OGRN] = *rec
	if rec.INN != "" {
		m.byINN[rec.INN] = *rec
	}
	return nil
}

func (m *memCache) GetFounders(_ context.Context, inn string) ([]domain.Founder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.founders[inn]; ok {
		m.hits++
		out := make([]domain.Founder, len(v))
		copy(out, v)
		return out, nil
	}
	return nil, nil
}

func (m *memCache) SetFounders(_ context.Context, inn string, founders []domain.Founder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]domain.Founder, len(founders))
	copy(cp, founders)
	m.founders[inn] = cp
	return nil
}

// --- helpers ---

func doGET(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		return rec, nil
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw=%s)", err, rec.Body.String())
	}
	return rec, body
}

// --- tests ---

func TestByINN_HappyPath_StubThenCache(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	// First call — cache miss → stub
	rec1, body1 := doGET(t, h, "/v1/egrul/by-inn/7707083893")
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	if body1["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", body1["source"])
	}
	data1 := body1["data"].(map[string]interface{})
	if data1["inn"] != "7707083893" {
		t.Fatalf("inn mismatch: %v", data1["inn"])
	}

	// Second call — cache hit
	rec2, body2 := doGET(t, h, "/v1/egrul/by-inn/7707083893")
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if body2["source"] != "cache" {
		t.Fatalf("second call should be cache, got %v", body2["source"])
	}
	if mc.hits == 0 {
		t.Fatal("expected at least one cache hit")
	}

	// Determinism: одинаковый ИНН → одинаковая запись
	data2 := body2["data"].(map[string]interface{})
	if data1["full_name"] != data2["full_name"] {
		t.Fatalf("non-deterministic: %v vs %v", data1["full_name"], data2["full_name"])
	}
}

func TestByINN_IndividualEntrepreneur(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	_, body := doGET(t, h, "/v1/egrul/by-inn/770708389355") // 12 digits
	data := body["data"].(map[string]interface{})
	if data["is_individual"] != true {
		t.Fatalf("expected individual entrepreneur, got %v", data["is_individual"])
	}
	if data["opf"] != "ИП" {
		t.Fatalf("expected ИП, got %v", data["opf"])
	}
}

func TestByINN_InvalidFormat(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	cases := []string{"123", "abcdefghij", "12345", "12345678901234567890"}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/v1/egrul/by-inn/"+c, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("input=%q expected 400, got %d", c, rec.Code)
		}
	}
}

func TestByOGRN_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	rec, body := doGET(t, h, "/v1/egrul/by-ogrn/1027700132195")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body["source"] != "stub" {
		t.Fatalf("expected stub, got %v", body["source"])
	}
	data := body["data"].(map[string]interface{})
	if data["ogrn"] != "1027700132195" {
		t.Fatalf("ogrn round-trip failed: %v", data["ogrn"])
	}

	// Cache hit on retry
	_, body2 := doGET(t, h, "/v1/egrul/by-ogrn/1027700132195")
	if body2["source"] != "cache" {
		t.Fatalf("expected cache on second call, got %v", body2["source"])
	}
}

func TestByOGRN_InvalidFormat(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	req := httptest.NewRequest(http.MethodGet, "/v1/egrul/by-ogrn/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFounders_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	rec, body := doGET(t, h, "/v1/egrul/founders/7707083893")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	data := body["data"].(map[string]interface{})

	count := int(data["count"].(float64))
	if count < 1 || count > 4 {
		t.Fatalf("expected 1..4 founders, got %d", count)
	}
	founders := data["founders"].([]interface{})
	if len(founders) != count {
		t.Fatalf("count mismatch: %d vs %d", count, len(founders))
	}

	// Сумма долей = 100.00%
	total := 0
	for _, f := range founders {
		fm := f.(map[string]interface{})
		var bps int
		var ipart, fpart int
		_, _ = parseDecimal(fm["share_percent"].(string), &ipart, &fpart)
		bps = ipart*100 + fpart
		total += bps
	}
	if total != 10000 {
		t.Fatalf("shares must sum to 100.00%%, got %d.%02d%%", total/100, total%100)
	}

	// Cache hit
	_, body2 := doGET(t, h, "/v1/egrul/founders/7707083893")
	if body2["source"] != "cache" {
		t.Fatalf("expected cache on second call, got %v", body2["source"])
	}
}

func TestFounders_IndividualReturnsEmpty(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	_, body := doGET(t, h, "/v1/egrul/founders/770708389355") // 12 digits → ИП
	data := body["data"].(map[string]interface{})
	if int(data["count"].(float64)) != 0 {
		t.Fatalf("expected 0 founders for ИП, got %v", data["count"])
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

// parseDecimal вытаскивает целую и дробную часть строки "12.34" в *int/*int.
func parseDecimal(s string, ipart, fpart *int) (bool, error) {
	*ipart = 0
	*fpart = 0
	dotSeen := false
	for _, c := range s {
		if c == '.' {
			dotSeen = true
			continue
		}
		if c < '0' || c > '9' {
			return false, nil
		}
		if !dotSeen {
			*ipart = *ipart*10 + int(c-'0')
		} else {
			*fpart = *fpart*10 + int(c-'0')
		}
	}
	return true, nil
}

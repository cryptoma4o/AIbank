package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"aibank/ext-rosfinmon/internal/domain"
)

// memCache — реализация cache.Cache для тестов.
type memCache struct {
	mu       sync.Mutex
	scr      map[string]domain.ScreeningResult
	snap     *domain.ListSnapshot
	scrHits  int
	snapHits int
}

func newMemCache() *memCache { return &memCache{scr: make(map[string]domain.ScreeningResult)} }

func (m *memCache) key(req domain.ScreeningRequest) string {
	return string(req.SubjectType) + "|" + req.Identifiers.INN + "|" + req.Identifiers.FullName + "|" + req.Identifiers.BirthDate
}

func (m *memCache) GetScreening(_ context.Context, req domain.ScreeningRequest) (*domain.ScreeningResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.scr[m.key(req)]; ok {
		m.scrHits++
		return &v, nil
	}
	return nil, nil
}

func (m *memCache) SetScreening(_ context.Context, req domain.ScreeningRequest, res *domain.ScreeningResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scr[m.key(req)] = *res
	return nil
}

func (m *memCache) GetSnapshot(_ context.Context) (*domain.ListSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snap != nil {
		m.snapHits++
		s := *m.snap
		return &s, nil
	}
	return nil, nil
}

func (m *memCache) SetSnapshot(_ context.Context, snap *domain.ListSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *snap
	m.snap = &cp
	return nil
}

// --- helpers ---

func doPOST(t *testing.T, h http.Handler, path string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	buf := &bytes.Buffer{}
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Body.Len() == 0 {
		return rec, nil
	}
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// --- tests ---

func TestScreen_HappyPath_NoMatch(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	req := domain.ScreeningRequest{
		SubjectType: domain.SubjectLegalEntity,
		Identifiers: domain.Identifiers{INN: "7707083893"},
	}
	rec, body := doPOST(t, h, "/v1/rosfinmon/screen", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", body["source"])
	}
	data := body["data"].(map[string]interface{})
	if data["matched"] != false {
		t.Fatalf("expected matched=false for fresh INN, got %v", data["matched"])
	}
	if data["request_id"] == "" || data["request_id"] == nil {
		t.Fatal("request_id must be set")
	}
}

func TestScreen_INNFromList_Matched(t *testing.T) {
	// Берём ИНН из синтетического перечня → точный матч с confidence=1.0
	mc := newMemCache()
	h := NewHandler(mc)

	list := domain.SyntheticList()
	if len(list) == 0 {
		t.Skip("empty synthetic list")
	}
	// Найдём одну запись ЮЛ
	var leINN string
	for _, e := range list {
		if e.INN != "" && len(e.INN) == 10 {
			leINN = e.INN
			break
		}
	}
	if leINN == "" {
		t.Skip("no LE entry in synthetic list")
	}

	req := domain.ScreeningRequest{
		SubjectType: domain.SubjectLegalEntity,
		Identifiers: domain.Identifiers{INN: leINN},
	}
	rec, body := doPOST(t, h, "/v1/rosfinmon/screen", req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	data := body["data"].(map[string]interface{})
	if data["matched"] != true {
		t.Fatalf("expected matched=true for known sanctioned INN %s", leINN)
	}
	conf := data["match_confidence"].(float64)
	if conf < 0.99 {
		t.Fatalf("expected confidence ~1.0, got %v", conf)
	}
	if !strings.HasPrefix(data["list_name"].(string), "115-FZ:") {
		t.Fatalf("unexpected list_name %v", data["list_name"])
	}
}

func TestScreen_CacheHit(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	req := domain.ScreeningRequest{
		SubjectType: domain.SubjectLegalEntity,
		Identifiers: domain.Identifiers{INN: "7707083893"},
	}
	doPOST(t, h, "/v1/rosfinmon/screen", req)
	_, body2 := doPOST(t, h, "/v1/rosfinmon/screen", req)
	if body2["source"] != "cache" {
		t.Fatalf("second call should be cache, got %v", body2["source"])
	}
	if mc.scrHits != 1 {
		t.Fatalf("expected 1 cache hit, got %d", mc.scrHits)
	}
}

func TestScreen_DeterministicMatch_AcrossManyINNs(t *testing.T) {
	// Проверяем, что синтетический ~1% даёт хотя бы одно совпадение на 1000 уникальных запросов.
	mc := newMemCache()
	h := NewHandler(mc)
	matches := 0
	const trials = 1000
	for i := 0; i < trials; i++ {
		req := domain.ScreeningRequest{
			SubjectType: domain.SubjectLegalEntity,
			// Уникальный ИНН на каждой итерации — гарантируем 1000 разных хэшей
			Identifiers: domain.Identifiers{
				INN:      fmt.Sprintf("770%07d", i),
				FullName: testName(i),
			},
		}
		_, body := doPOST(t, h, "/v1/rosfinmon/screen", req)
		data := body["data"].(map[string]interface{})
		if data["matched"] == true {
			matches++
		}
	}
	if matches == 0 {
		t.Fatalf("expected at least one synthetic match across %d trials, got 0", trials)
	}
	// Ожидаем ~1%; верхняя граница с большим запасом, чтобы тест был стабильным.
	if matches > trials/20 {
		t.Fatalf("match rate too high: %d/%d (~%d%%) — expected ~1%%", matches, trials, matches*100/trials)
	}
}

func TestScreen_InvalidRequest(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)

	cases := []domain.ScreeningRequest{
		{SubjectType: "", Identifiers: domain.Identifiers{INN: "7707083893"}},  // bad subject
		{SubjectType: domain.SubjectLegalEntity, Identifiers: domain.Identifiers{}}, // no ids
		{SubjectType: domain.SubjectLegalEntity, Identifiers: domain.Identifiers{INN: "12"}}, // wrong INN length
	}
	for i, c := range cases {
		rec, _ := doPOST(t, h, "/v1/rosfinmon/screen", c)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: expected 400, got %d", i, rec.Code)
		}
	}
}

func TestScreen_BadJSON(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	req := httptest.NewRequest(http.MethodPost, "/v1/rosfinmon/screen", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListInfo_HappyPath(t *testing.T) {
	mc := newMemCache()
	h := NewHandler(mc)
	req := httptest.NewRequest(http.MethodGet, "/v1/rosfinmon/list-info", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["source"] != "stub" {
		t.Fatalf("first call should be stub, got %v", out["source"])
	}
	data := out["data"].(map[string]interface{})
	if int(data["list_count"].(float64)) <= 0 {
		t.Fatalf("list_count must be > 0")
	}

	// Second call — cache hit
	req2 := httptest.NewRequest(http.MethodGet, "/v1/rosfinmon/list-info", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	var out2 map[string]interface{}
	_ = json.Unmarshal(rec2.Body.Bytes(), &out2)
	if out2["source"] != "cache" {
		t.Fatalf("expected cache, got %v", out2["source"])
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

func testName(i int) string {
	names := []string{"Альфа", "Бета", "Гамма", "Дельта", "Сигма"}
	return names[i%len(names)] + " " + namesPart(i)
}

func namesPart(i int) string {
	letters := "АБВГДЕЖЗИК"
	return string([]rune(letters)[i%len([]rune(letters))])
}

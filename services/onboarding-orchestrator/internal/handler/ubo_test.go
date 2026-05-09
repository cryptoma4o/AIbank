package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
)

type fakeUBORepo struct {
	mu     sync.Mutex
	graphs map[string]*domain.UBOGraph // key = tenantID + ":" + applicationID
}

func newFakeUBORepo() *fakeUBORepo {
	return &fakeUBORepo{graphs: map[string]*domain.UBOGraph{}}
}

func (f *fakeUBORepo) Upsert(_ context.Context, g *domain.UBOGraph) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.graphs[g.TenantID+":"+g.ApplicationID] = g
	return nil
}

func (f *fakeUBORepo) GetByApplication(_ context.Context, tenantID, applicationID string) (*domain.UBOGraph, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.graphs[tenantID+":"+applicationID]; ok {
		return g, nil
	}
	return nil, repository.ErrNotFound
}

func newUBOHandlerForTest() (http.Handler, *fakeUBORepo) {
	repo := newFakeUBORepo()
	h := NewUBOHandler(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h.Routes(), repo
}

func postUBO(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestUBO_HappyPath — один UBO-узел с долей 50%, FATCA US-person → ok.
func TestUBO_HappyPath(t *testing.T) {
	h, repo := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes": []map[string]any{
			{
				"id":              "n1",
				"node_type":       "person",
				"name":            "Иванов И.И.",
				"direct_stake":    50,
				"effective_stake": 50,
				"is_ubo":          true,
				"control_basis":   "capital_share",
				"fatca_declaration": map[string]any{
					"us_person":               false,
					"tax_residency_countries": []string{"RU"},
				},
			},
		},
		"edges": []map[string]any{
			{"from_node_id": "n1", "to_node_id": "le_x", "stake": 50},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	if got, _ := repo.GetByApplication(context.Background(), "demo", "app_x"); got == nil || len(got.Nodes) != 1 {
		t.Fatalf("repo missing graph or wrong nodes count: %+v", got)
	}
}

// TestUBO_NoUBOReason — UBO не определены, но указано обоснование → ok.
func TestUBO_NoUBOReason(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes":           []map[string]any{},
		"edges":           []map[string]any{},
		"no_ubo_reason":   "Распылённое владение, нет физлица с долей ≥25%",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
}

// TestUBO_EIOAsUBO — confirm что ЕИО признан UBO → ok без узлов.
func TestUBO_EIOAsUBO(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_x",
		"legal_entity_id":         "le_x",
		"nodes":                   []map[string]any{},
		"edges":                   []map[string]any{},
		"eio_as_ubo_confirmation": true,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
}

// TestUBO_MissingUBOInfo — нет UBO, нет обоснования, нет EIO confirmation → 400.
func TestUBO_MissingUBOInfo(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes":           []map[string]any{},
		"edges":           []map[string]any{},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (115-FZ requires UBO info)", rr.Code)
	}
}

// TestUBO_BadStake — direct_stake > 100 → 400.
func TestUBO_BadStake(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes": []map[string]any{
			{"id": "n1", "node_type": "person", "name": "X", "direct_stake": 150, "effective_stake": 50, "is_ubo": true},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (stake>100)", rr.Code)
	}
}

// TestUBO_BadNodeType — node_type=invalid → 400.
func TestUBO_BadNodeType(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes": []map[string]any{
			{"id": "n1", "node_type": "alien", "name": "X", "direct_stake": 50, "effective_stake": 50, "is_ubo": true},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad node_type)", rr.Code)
	}
}

// TestUBO_BadControlBasis — control_basis=verbal → 400.
func TestUBO_BadControlBasis(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	rr := postUBO(t, h, map[string]any{
		"tenant_id":       "demo",
		"application_id":  "app_x",
		"legal_entity_id": "le_x",
		"nodes": []map[string]any{
			{"id": "n1", "node_type": "person", "name": "X", "direct_stake": 30, "effective_stake": 30, "is_ubo": true, "control_basis": "verbal"},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (bad control_basis)", rr.Code)
	}
}

// TestUBO_GetByApplication — после upsert GET возвращает граф.
func TestUBO_GetByApplication(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	postUBO(t, h, map[string]any{
		"tenant_id":               "demo",
		"application_id":          "app_y",
		"legal_entity_id":         "le_y",
		"nodes":                   []map[string]any{},
		"edges":                   []map[string]any{},
		"eio_as_ubo_confirmation": true,
	})
	req := httptest.NewRequest(http.MethodGet, "/by-application/app_y?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var got domain.UBOGraph
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if !got.EIOAsUBOConfirmation {
		t.Errorf("eio_as_ubo_confirmation should be true")
	}
}

// TestUBO_GetNotFound — 404 на несуществующую заявку.
func TestUBO_GetNotFound(t *testing.T) {
	h, _ := newUBOHandlerForTest()
	req := httptest.NewRequest(http.MethodGet, "/by-application/missing?tenant_id=demo", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

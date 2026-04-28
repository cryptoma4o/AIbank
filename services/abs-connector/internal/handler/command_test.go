package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"aibank/abs-connector/internal/clients"
	"aibank/abs-connector/internal/dedupstore"
	"aibank/abs-connector/internal/domain"
	"aibank/abs-connector/internal/handler"
)

const (
	testIdemKey = "11111111-2222-3333-4444-555555555555"
	testTenant  = "bank-alpha"
)

// setupServer поднимает (a) mock-adapter httptest.Server с настраиваемым
// counter'ом вызовов, (b) connector handler, (c) connector httptest.Server.
// Возвращает url connector'а и pointer на счётчик.
// Cleanup регистрируется через t.Cleanup, чтобы корректно работать
// с t.Parallel() в подтестах (parent-level defer закрывал бы серверы
// до того, как подтесты успевали выполниться).
func setupServer(t *testing.T, registerTenant bool) (connectorURL string, adapterCalls *int32) {
	t.Helper()

	var calls int32
	adapterSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/execute" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		var req domain.AdapterRequest
		_ = json.Unmarshal(body, &req)

		resp := domain.AdapterResponse{
			IdempotencyKey: req.IdempotencyKey,
			Success:        true,
			Data:           json.RawMessage(`{"account_number":"40702810999900007777"}`),
			AdapterUsed:    "abs-adapter-cft",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	registry := domain.NewAdapterRegistry()
	if registerTenant {
		if err := registry.Register(domain.AdapterEntry{
			TenantID:    testTenant,
			AdapterName: "cft",
			URL:         adapterSrv.URL,
			Version:     "1.4.2",
		}); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	store := dedupstore.NewInMemoryStore()
	client := clients.NewAdapterClient(0)

	h := handler.New(handler.Config{
		Registry: registry,
		Store:    store,
		Client:   client,
	})

	connectorSrv := httptest.NewServer(h.Router())

	t.Cleanup(func() {
		adapterSrv.Close()
		connectorSrv.Close()
	})

	return connectorSrv.URL, &calls
}

func postCommand(t *testing.T, baseURL string, payload map[string]any) (*http.Response, []byte) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/commands", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func TestExecute_FirstCallHitsAdapterAndStoresResponse(t *testing.T) {
	t.Parallel()

	url, calls := setupServer(t, true)

	payload := map[string]any{
		"idempotency_key": testIdemKey,
		"tenant_id":       testTenant,
		"command":         string(domain.CmdOpenAccount),
		"payload":         json.RawMessage(`{"client_id":"c-1","account_type":"current_rub"}`),
		"metadata": map[string]any{
			"trace_id":    "trace-001",
			"initiated_by": "wf-open-account",
		},
	}
	resp, body := postCommand(t, url, payload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}

	var canonical domain.CanonicalResponse
	if err := json.Unmarshal(body, &canonical); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if !canonical.Success {
		t.Fatalf("expected success, got %+v", canonical)
	}
	if canonical.AdapterUsed != "cft" {
		t.Fatalf("adapter_used = %q, want cft", canonical.AdapterUsed)
	}
	if canonical.AdapterVersion != "1.4.2" {
		t.Fatalf("adapter_version = %q, want 1.4.2", canonical.AdapterVersion)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("adapter calls=%d, want 1", got)
	}
}

func TestExecute_ReplayServesFromCacheWithoutCallingAdapter(t *testing.T) {
	t.Parallel()

	url, calls := setupServer(t, true)

	payload := map[string]any{
		"idempotency_key": testIdemKey,
		"tenant_id":       testTenant,
		"command":         string(domain.CmdOpenAccount),
		"payload":         json.RawMessage(`{"client_id":"c-1"}`),
	}
	if resp, body := postCommand(t, url, payload); resp.StatusCode != 200 {
		t.Fatalf("first call: status=%d body=%s", resp.StatusCode, body)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("after first call, adapter calls=%d, want 1", got)
	}

	// Второй вызов с тем же idempotency_key — adapter трогать не должны.
	if resp, body := postCommand(t, url, payload); resp.StatusCode != 200 {
		t.Fatalf("second call: status=%d body=%s", resp.StatusCode, body)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("after replay, adapter calls=%d, want still 1 (cache hit)", got)
	}
}

func TestExecute_ReturnsServiceUnavailableWhenAdapterMissing(t *testing.T) {
	t.Parallel()

	url, calls := setupServer(t, false /* tenant NOT registered */)

	payload := map[string]any{
		"idempotency_key": testIdemKey,
		"tenant_id":       testTenant,
		"command":         string(domain.CmdOpenAccount),
		"payload":         json.RawMessage(`{}`),
	}
	resp, body := postCommand(t, url, payload)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "adapter_not_configured") {
		t.Fatalf("expected adapter_not_configured in body, got %s", body)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("expected 0 adapter calls, got %d", got)
	}
}

func TestExecute_ValidationErrors(t *testing.T) {
	t.Parallel()

	url, _ := setupServer(t, true)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"missing idempotency_key", map[string]any{
			"tenant_id": testTenant, "command": "OpenAccount", "payload": json.RawMessage(`{}`),
		}},
		{"non-uuid idempotency_key", map[string]any{
			"idempotency_key": "not-a-uuid", "tenant_id": testTenant,
			"command": "OpenAccount", "payload": json.RawMessage(`{}`),
		}},
		{"invalid tenant_id", map[string]any{
			"idempotency_key": testIdemKey, "tenant_id": "Bad Tenant!",
			"command": "OpenAccount", "payload": json.RawMessage(`{}`),
		}},
		{"unknown command", map[string]any{
			"idempotency_key": testIdemKey, "tenant_id": testTenant,
			"command": "TransferMoney", "payload": json.RawMessage(`{}`),
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, body := postCommand(t, url, tc.payload)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, body)
			}
		})
	}
}

func TestGetCommand_HitAndMiss(t *testing.T) {
	t.Parallel()

	url, _ := setupServer(t, true)

	// Сначала миссит.
	getURL := url + "/v1/commands/" + testIdemKey + "?tenant_id=" + testTenant
	resp, body := getJSON(t, getURL)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pre-execute GET: status=%d body=%s", resp.StatusCode, body)
	}

	// Выполняем команду.
	if r, b := postCommand(t, url, map[string]any{
		"idempotency_key": testIdemKey,
		"tenant_id":       testTenant,
		"command":         string(domain.CmdOpenAccount),
		"payload":         json.RawMessage(`{}`),
	}); r.StatusCode != 200 {
		t.Fatalf("execute: status=%d body=%s", r.StatusCode, b)
	}

	// Теперь должен быть hit.
	resp, body = getJSON(t, getURL)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-execute GET: status=%d body=%s", resp.StatusCode, body)
	}
	var canonical domain.CanonicalResponse
	if err := json.Unmarshal(body, &canonical); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if canonical.IdempotencyKey != testIdemKey || !canonical.Success {
		t.Fatalf("unexpected response: %+v", canonical)
	}
}

func TestGetCommand_BadIdempotencyKey(t *testing.T) {
	t.Parallel()

	url, _ := setupServer(t, true)

	resp, _ := getJSON(t, url+"/v1/commands/not-a-uuid?tenant_id="+testTenant)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHealthAndReady(t *testing.T) {
	t.Parallel()

	url, _ := setupServer(t, true)

	for _, path := range []string{"/health", "/healthz", "/ready"} {
		resp, _ := getJSON(t, url+path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d", path, resp.StatusCode)
		}
	}
}

func getJSON(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aibank/abs-adapter-cft/internal/domain"
)

// helper — POST /v1/execute с указанной командой.
func postExecute(t *testing.T, h http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/execute", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeResp(t *testing.T, rr *httptest.ResponseRecorder) domain.ABSResponse {
	t.Helper()
	var resp domain.ABSResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return resp
}

func TestExecute_OpenAccount_ReturnsValid20DigitRubAccount(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "open-account-001",
		TenantID:       "bank-alpha",
		Command:        domain.CmdOpenAccount,
		Payload:        map[string]any{"client_id": "cft_demo", "account_type": "current_rub"},
	}

	rr := postExecute(t, h, cmd)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	resp := decodeResp(t, rr)
	if !resp.Success {
		t.Fatalf("expected success, got error=%q", resp.Error)
	}

	acc, ok := resp.Data["account_number"].(string)
	if !ok || acc == "" {
		t.Fatalf("expected account_number in data, got %#v", resp.Data)
	}
	if !strings.HasPrefix(acc, "40702810") {
		t.Fatalf("expected account_number to start with 40702810, got %q", acc)
	}
	if len(acc) != 20 {
		t.Fatalf("expected 20-digit account_number, got %d (%q)", len(acc), acc)
	}
	for _, ch := range acc {
		if ch < '0' || ch > '9' {
			t.Fatalf("expected pure-digit account_number, got %q", acc)
		}
	}
	if resp.AdapterUsed != "abs-adapter-cft" {
		t.Fatalf("unexpected adapter_used: %q", resp.AdapterUsed)
	}
	if resp.IdempotencyKey != cmd.IdempotencyKey {
		t.Fatalf("idempotency_key roundtrip failed: %q", resp.IdempotencyKey)
	}
}

func TestExecute_OpenAccount_DeterministicBySameKey(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "open-account-deterministic",
		Command:        domain.CmdOpenAccount,
	}
	first := decodeResp(t, postExecute(t, h, cmd))
	second := decodeResp(t, postExecute(t, h, cmd))
	if first.Data["account_number"] != second.Data["account_number"] {
		t.Fatalf("expected deterministic account_number, got %v vs %v",
			first.Data["account_number"], second.Data["account_number"])
	}
}

func TestExecute_CreateClient_PrefixCft(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "create-client-001",
		Command:        domain.CmdCreateClient,
	}
	rr := postExecute(t, h, cmd)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	resp := decodeResp(t, rr)
	if !resp.Success {
		t.Fatalf("expected success, got %q", resp.Error)
	}
	clientID, ok := resp.Data["client_id"].(string)
	if !ok || !strings.HasPrefix(clientID, "cft_") {
		t.Fatalf("expected client_id with cft_ prefix, got %#v", resp.Data)
	}
}

func TestExecute_GetAccountInfo(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "info-001",
		Command:        domain.CmdGetAccountInfo,
	}
	rr := postExecute(t, h, cmd)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	resp := decodeResp(t, rr)
	if !resp.Success {
		t.Fatalf("expected success, got %q", resp.Error)
	}
	if _, ok := resp.Data["balance_kopecks"]; !ok {
		t.Fatalf("expected balance_kopecks, got %#v", resp.Data)
	}
	if resp.Data["status"] != "active" {
		t.Fatalf("expected status=active, got %#v", resp.Data["status"])
	}
}

func TestExecute_CloseAccount(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "close-001",
		Command:        domain.CmdCloseAccount,
	}
	rr := postExecute(t, h, cmd)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	resp := decodeResp(t, rr)
	if !resp.Success {
		t.Fatalf("expected success, got %q", resp.Error)
	}
	if resp.Data["status"] != "closed" {
		t.Fatalf("expected status=closed, got %#v", resp.Data["status"])
	}
}

func TestExecute_UnknownCommand_ReturnsFailureNotHTTP500(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	cmd := domain.ABSCommand{
		IdempotencyKey: "unknown-001",
		Command:        domain.CommandType("FrobnicateAccount"),
	}
	rr := postExecute(t, h, cmd)
	// Контракт: неизвестная команда — это бизнес-failure (Success=false),
	// не HTTP-ошибка, чтобы connector мог записать в dedup-store.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (business failure), got %d", rr.Code)
	}
	resp := decodeResp(t, rr)
	if resp.Success {
		t.Fatalf("expected success=false for unknown command")
	}
	if !strings.Contains(resp.Error, "FrobnicateAccount") {
		t.Fatalf("expected error to mention command name, got %q", resp.Error)
	}
}

func TestExecute_InvalidBody_Returns400(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	req := httptest.NewRequest(http.MethodPost, "/v1/execute",
		bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestHealthz_Returns200(t *testing.T) {
	h := NewHandler("1.0.0").Router()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestVersion_ReturnsAdapterMetadata(t *testing.T) {
	h := NewHandler("1.4.2").Router()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["adapter"] != "cft" {
		t.Fatalf("expected adapter=cft, got %v", payload["adapter"])
	}
	if payload["version"] != "1.4.2" {
		t.Fatalf("expected version=1.4.2, got %v", payload["version"])
	}
	cmds, ok := payload["commands"].([]any)
	if !ok || len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %#v", payload["commands"])
	}
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"aibank/onboarding-orchestrator/internal/extclients"
)

// fakePrequalifyClient — управляемый stub под интерфейс PrequalifyClient.
type fakePrequalifyClient struct {
	egrul    *extclients.EGRULData
	egrulErr error
	rfm      *extclients.RosfinmonResult
	rfmErr   error
	fssp     *extclients.FSSPResult
	fsspErr  error
}

func (f *fakePrequalifyClient) EGRULByINN(_ context.Context, _ string) (*extclients.EGRULData, error) {
	return f.egrul, f.egrulErr
}
func (f *fakePrequalifyClient) RosfinmonScreen(_ context.Context, _ string) (*extclients.RosfinmonResult, error) {
	return f.rfm, f.rfmErr
}
func (f *fakePrequalifyClient) FSSPByINN(_ context.Context, _ string) (*extclients.FSSPResult, error) {
	return f.fssp, f.fsspErr
}

func newPrequalifyHandlerWith(c PrequalifyClient) *PrequalifyHandler {
	return NewPrequalifyHandler(c, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func postPrequalify(t *testing.T, h *PrequalifyHandler, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/prequalify", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Prequalify(rr, req)
	return rr
}

// TestPrequalify_Proceed — все источники чистые, имя совпадает → proceed.
func TestPrequalify_Proceed(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul: &extclients.EGRULData{
			INN: "7707083893", OGRN: "1027700132195",
			FullName:     "Общество с ограниченной ответственностью «Ромашка»",
			ShortName:    "ООО «Ромашка»",
			Status:       "active",
			RegisteredAt: "2010-01-15",
			Address:      "Москва",
			CEO:          "Иванов И.И.",
		},
		rfm:  &extclients.RosfinmonResult{Matched: false},
		fssp: &extclients.FSSPResult{Count: 0},
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "Ромашка",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var resp PrequalifyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Decision != "proceed" {
		t.Fatalf("decision=%q reason=%q want proceed", resp.Decision, resp.DecisionReason)
	}
	if !resp.NameMatchesEGRUL {
		t.Errorf("expected name match for 'Ромашка' vs %q", c.egrul.ShortName)
	}
}

// TestPrequalify_RejectOnRosfinmon — match в Росфинмоне → reject.
func TestPrequalify_RejectOnRosfinmon(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul: &extclients.EGRULData{Status: "active", ShortName: "ООО Тест", FullName: "Тест"},
		rfm:   &extclients.RosfinmonResult{Matched: true, ListName: "115-FZ:terrorist"},
		fssp:  &extclients.FSSPResult{},
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "Тест",
	})
	var resp PrequalifyResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Decision != "reject" {
		t.Fatalf("decision=%q want reject (rosfinmon match)", resp.Decision)
	}
	if !resp.RosfinmonPresent {
		t.Errorf("rosfinmon_present should be true")
	}
}

// TestPrequalify_RejectOnLiquidated — ЕГРЮЛ status != active → reject.
func TestPrequalify_RejectOnLiquidated(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul: &extclients.EGRULData{Status: "liquidated", ShortName: "ООО Старт", FullName: "Старт"},
		rfm:   &extclients.RosfinmonResult{Matched: false},
		fssp:  &extclients.FSSPResult{},
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "Старт",
	})
	var resp PrequalifyResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Decision != "reject" {
		t.Fatalf("decision=%q want reject (egrul liquidated)", resp.Decision)
	}
}

// TestPrequalify_ManualOnNameMismatch — не совпало имя → manual_review.
func TestPrequalify_ManualOnNameMismatch(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul: &extclients.EGRULData{Status: "active", ShortName: "ООО Ромашка", FullName: "Ромашка"},
		rfm:   &extclients.RosfinmonResult{Matched: false},
		fssp:  &extclients.FSSPResult{},
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "Совершенно другое",
	})
	var resp PrequalifyResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Decision != "manual_review" {
		t.Fatalf("decision=%q want manual_review (name mismatch)", resp.Decision)
	}
}

// TestPrequalify_ManualOnFSSP — много исполнительных производств → manual_review.
func TestPrequalify_ManualOnFSSP(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul: &extclients.EGRULData{Status: "active", ShortName: "ООО Должник", FullName: "Должник"},
		rfm:   &extclients.RosfinmonResult{Matched: false},
		fssp:  &extclients.FSSPResult{Count: 42, TotalDebtKopecks: 1_000_000_00},
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "Должник",
	})
	var resp PrequalifyResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Decision != "manual_review" {
		t.Fatalf("decision=%q want manual_review (fssp>10)", resp.Decision)
	}
	if resp.FSSPProceedingsCount != 42 {
		t.Errorf("fssp count = %d want 42", resp.FSSPProceedingsCount)
	}
}

// TestPrequalify_PartialAvailability — один источник упал, остальные работают → proceed
// + unavailable_sources содержит упавший источник.
func TestPrequalify_PartialAvailability(t *testing.T) {
	c := &fakePrequalifyClient{
		egrul:   &extclients.EGRULData{Status: "active", ShortName: "ООО ОК", FullName: "ОК"},
		rfm:     &extclients.RosfinmonResult{Matched: false},
		fsspErr: errors.New("connection refused"),
	}
	rr := postPrequalify(t, newPrequalifyHandlerWith(c), map[string]string{
		"tenant_id": "demo", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "ОК",
	})
	var resp PrequalifyResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Decision != "proceed" {
		t.Fatalf("decision=%q want proceed (fssp unavailable, остальные чистые)", resp.Decision)
	}
	if len(resp.UnavailableSources) != 1 || resp.UnavailableSources[0] != "fssp" {
		t.Fatalf("unavailable_sources = %v want [fssp]", resp.UnavailableSources)
	}
}

// TestPrequalify_BadINN — 9 digits → 400 validation_failed.
func TestPrequalify_BadINN(t *testing.T) {
	rr := postPrequalify(t, newPrequalifyHandlerWith(&fakePrequalifyClient{}), map[string]string{
		"tenant_id": "demo", "inn": "770708389", "ogrn": "1027700132195", "short_name": "X",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

// TestPrequalify_BadTenant — невалидный tenant_id → 400.
func TestPrequalify_BadTenant(t *testing.T) {
	rr := postPrequalify(t, newPrequalifyHandlerWith(&fakePrequalifyClient{}), map[string]string{
		"tenant_id": "BAD-id!", "inn": "7707083893", "ogrn": "1027700132195", "short_name": "X",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rr.Code)
	}
}

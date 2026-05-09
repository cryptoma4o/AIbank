package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aibank/platform/services/document-service/internal/domain"
)

// seedDoc добавляет один документ в fakeRepo (минимальный набор полей).
func seedDoc(repo *fakeRepo, tenantID, applicationID, docType, id string) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.docs[id] = &domain.Document{
		ID:            id,
		TenantID:      tenantID,
		ApplicationID: applicationID,
		Type:          domain.DocumentType(docType),
		State:         domain.DocumentStateUploaded,
		FileID:        "file_" + id,
		Filename:      docType + ".pdf",
		MimeType:      "application/pdf",
		SizeBytes:     1024,
		SHA256:        strings.Repeat("a", 64),
		StoragePath:   "memory://documents/" + id,
		SourceType:    domain.SourceClientUpload,
		UploadedAt:    time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func getChecklist(t *testing.T, h *DocumentHandler, tenantID, applicationID, form string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/checklist?tenant_id=" + tenantID +
		"&application_id=" + applicationID +
		"&legal_entity_form=" + form
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	return rr
}

// TestChecklist_LLC_Empty — для LLC без загруженных документов чеклист
// показывает 8 missing.
func TestChecklist_LLC_Empty(t *testing.T) {
	h, _, _ := newTestHandler()
	rr := getChecklist(t, h, "demo", "app_x", "LLC")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var resp struct {
		LegalEntityForm string `json:"legal_entity_form"`
		Required        []struct {
			Type       string `json:"type"`
			Status     string `json:"status"`
			DocumentID string `json:"document_id,omitempty"`
		} `json:"required"`
		MissingCount int  `json:"missing_count"`
		Complete     bool `json:"complete"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.LegalEntityForm != "LLC" {
		t.Errorf("form=%q want LLC", resp.LegalEntityForm)
	}
	if len(resp.Required) != 8 {
		t.Errorf("required len=%d want 8 (LLC)", len(resp.Required))
	}
	if resp.MissingCount != 8 {
		t.Errorf("missing=%d want 8", resp.MissingCount)
	}
	if resp.Complete {
		t.Error("complete should be false on empty")
	}
}

// TestChecklist_LLC_Partial — загружено 2 из 8 → status loaded для них,
// missing для остальных.
func TestChecklist_LLC_Partial(t *testing.T) {
	h, repo, _ := newTestHandler()
	seedDoc(repo, "demo", "app_x", "charter", "doc_charter_1")
	seedDoc(repo, "demo", "app_x", "egrul_record_sheet", "doc_egrul_1")

	rr := getChecklist(t, h, "demo", "app_x", "LLC")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body)
	}
	var resp struct {
		Required []struct {
			Type       string `json:"type"`
			Status     string `json:"status"`
			DocumentID string `json:"document_id,omitempty"`
		} `json:"required"`
		MissingCount int  `json:"missing_count"`
		Complete     bool `json:"complete"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	loaded := map[string]string{}
	for _, item := range resp.Required {
		if item.Status == "loaded" {
			loaded[item.Type] = item.DocumentID
		}
	}
	if loaded["charter"] != "doc_charter_1" {
		t.Errorf("charter not loaded properly: %v", loaded)
	}
	if loaded["egrul_record_sheet"] != "doc_egrul_1" {
		t.Errorf("egrul_record_sheet not loaded properly: %v", loaded)
	}
	if resp.MissingCount != 6 {
		t.Errorf("missing=%d want 6", resp.MissingCount)
	}
	if resp.Complete {
		t.Error("complete should be false")
	}
}

// TestChecklist_IP — для ИП требуется 4 документа.
func TestChecklist_IP(t *testing.T) {
	h, _, _ := newTestHandler()
	rr := getChecklist(t, h, "demo", "app_y", "IP")
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp struct {
		Required []struct{ Type string }
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Required) != 4 {
		t.Errorf("IP required len=%d want 4", len(resp.Required))
	}
}

// TestChecklist_Complete — все 4 документа ИП загружены → complete=true.
func TestChecklist_Complete(t *testing.T) {
	h, repo, _ := newTestHandler()
	seedDoc(repo, "demo", "app_y", "passport", "d1")
	seedDoc(repo, "demo", "app_y", "inn_certificate", "d2")
	seedDoc(repo, "demo", "app_y", "ogrn_certificate", "d3")
	seedDoc(repo, "demo", "app_y", "signature_card", "d4")

	rr := getChecklist(t, h, "demo", "app_y", "IP")
	var resp struct {
		MissingCount int  `json:"missing_count"`
		Complete     bool `json:"complete"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.MissingCount != 0 || !resp.Complete {
		t.Errorf("missing=%d complete=%v want 0/true", resp.MissingCount, resp.Complete)
	}
}

// TestChecklist_BadForm — invalid legal_entity_form → 400.
func TestChecklist_BadForm(t *testing.T) {
	h, _, _ := newTestHandler()
	rr := getChecklist(t, h, "demo", "app_y", "ALIEN")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

// TestChecklist_MissingApplicationID — нет application_id → 400.
func TestChecklist_MissingApplicationID(t *testing.T) {
	h, _, _ := newTestHandler()
	url := "/checklist?tenant_id=demo&legal_entity_form=LLC"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

// TestChecklist_MissingTenant — нет tenant_id → 400.
func TestChecklist_MissingTenant(t *testing.T) {
	h, _, _ := newTestHandler()
	url := "/checklist?application_id=app_x&legal_entity_form=LLC"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

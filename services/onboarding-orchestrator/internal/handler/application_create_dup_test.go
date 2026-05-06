package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"aibank/onboarding-orchestrator/internal/domain"
)

// counterIDGen генерирует app_001, app_002, ... — нужен для тестов
// с несколькими создаваемыми заявками; fixedIDGen вернул бы один и тот же ID.
type counterIDGen struct {
	n atomic.Int64
}

func (g *counterIDGen) NewApplicationID() string {
	v := g.n.Add(1)
	return "app_dup_" + strconv.FormatInt(v, 10)
}

// dupTestSetup поднимает chi-роутер с реальным ApplicationHandler и
// counterIDGen + fakeTemporal (executeCalls не проверяем здесь). Audit
// disabled (nil), чтобы не тащить httptest сервер ради двух Create-вызовов.
func dupTestSetup(t *testing.T) (http.Handler, *fakeAppRepo) {
	t.Helper()
	repo := newFakeAppRepo()
	tc := &fakeTemporal{}
	idgen := &counterIDGen{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewApplicationHandler(repo, tc, idgen, nil, log)
	r := h.Routes()
	return r, repo
}

func mustCreate(t *testing.T, h http.Handler, tenantID, applicantID string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"tenant_id":         tenantID,
		"applicant_id":      applicantID,
		"legal_entity_type": domain.LegalEntityLLC,
		"channel":           domain.ChannelWeb,
		"product_codes":     []string{"current_account"},
		"risk_thresholds": map[string]float64{
			"auto_approve_below": 0.30,
			"decline_above":      0.85,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestCreate_DuplicateApplicant_Returns409 — повторный POST с тем же
// (tenant_id, applicant_id) возвращает 409 + existing_application_id первого.
func TestCreate_DuplicateApplicant_Returns409(t *testing.T) {
	h, repo := dupTestSetup(t)

	rr1 := mustCreate(t, h, "demo", "usr_dup_test")
	if rr1.Code != http.StatusCreated {
		t.Fatalf("первый Create: status=%d body=%s", rr1.Code, rr1.Body)
	}
	var resp1 struct {
		ApplicationID string `json:"application_id"`
	}
	_ = json.Unmarshal(rr1.Body.Bytes(), &resp1)
	if resp1.ApplicationID == "" {
		t.Fatalf("первый Create вернул пустой application_id: %s", rr1.Body)
	}

	// Sanity: репозиторий действительно содержит запись.
	if got, err := repo.GetByApplicant(context.Background(), "demo", "usr_dup_test"); err != nil || got.ID != resp1.ApplicationID {
		t.Fatalf("после Create репо вернуло err=%v got=%+v want=%s", err, got, resp1.ApplicationID)
	}

	rr2 := mustCreate(t, h, "demo", "usr_dup_test")
	if rr2.Code != http.StatusConflict {
		t.Fatalf("повторный Create должен дать 409, получено %d body=%s", rr2.Code, rr2.Body)
	}
	var conflict struct {
		Error                 map[string]string `json:"error"`
		ExistingApplicationID string            `json:"existing_application_id"`
		ExistingState         string            `json:"existing_state"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("unmarshal 409: %v body=%s", err, rr2.Body)
	}
	if conflict.Error["code"] != "applicant_has_application" {
		t.Errorf("code = %q, want applicant_has_application", conflict.Error["code"])
	}
	if conflict.ExistingApplicationID != resp1.ApplicationID {
		t.Errorf("existing_application_id = %q, want %q", conflict.ExistingApplicationID, resp1.ApplicationID)
	}
	if conflict.ExistingState != string(domain.StateDraft) {
		t.Errorf("existing_state = %q, want %s", conflict.ExistingState, domain.StateDraft)
	}
}

// TestCreate_DifferentApplicants_BothSucceed — sanity, что правило не задевает
// разных applicant'ов в одном тенанте.
func TestCreate_DifferentApplicants_BothSucceed(t *testing.T) {
	h, _ := dupTestSetup(t)

	rr1 := mustCreate(t, h, "demo", "usr_dup_a")
	if rr1.Code != http.StatusCreated {
		t.Fatalf("applicant A: status=%d body=%s", rr1.Code, rr1.Body)
	}
	rr2 := mustCreate(t, h, "demo", "usr_dup_b")
	if rr2.Code != http.StatusCreated {
		t.Fatalf("applicant B: status=%d body=%s", rr2.Code, rr2.Body)
	}
}

// TestCreate_SameApplicantDifferentTenants_BothSucceed — sanity, что правило
// scoped per tenant: один applicant_id может существовать в разных тенантах.
func TestCreate_SameApplicantDifferentTenants_BothSucceed(t *testing.T) {
	h, _ := dupTestSetup(t)

	rr1 := mustCreate(t, h, "demo", "usr_shared")
	if rr1.Code != http.StatusCreated {
		t.Fatalf("tenant demo: status=%d body=%s", rr1.Code, rr1.Body)
	}
	rr2 := mustCreate(t, h, "bank_alpha", "usr_shared")
	if rr2.Code != http.StatusCreated {
		t.Fatalf("tenant bank_alpha: status=%d body=%s", rr2.Code, rr2.Body)
	}
}

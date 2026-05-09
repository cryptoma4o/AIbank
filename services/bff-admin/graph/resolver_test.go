package graph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/graphql-go/graphql"

	"aibank/bff-admin/internal/auth"
	"aibank/bff-admin/internal/clients"
)

func stubServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, body := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func newTestResolver(t *testing.T, srvURL string) *Resolver {
	t.Helper()
	return NewResolver(
		clients.NewTenantClient(srvURL),
		clients.NewAuditClient(srvURL),
		clients.NewOrchestratorClient(srvURL),
		clients.NewRiskClient(srvURL),
		clients.NewIdentityClient(srvURL),
	)
}

func adminCtx(role string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u_admin", TenantID: "alpha", Role: role,
	})
}

func TestSchema_Builds(t *testing.T) {
	r := newTestResolver(t, "http://unused")
	if _, err := r.Schema(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
}

func TestQuery_Application(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/applications/app_1": `{
			"id":"app_1","tenant_id":"alpha","applicant_id":"u_2","legal_entity_type":"LLC",
			"channel":"web","state":"manual_review","product_codes":["current_rub"],
			"workflow_id":"wf","created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"
		}`,
		"/v1/risk-assessments": `{
			"id":"r_1","application_id":"app_1","score":0.42,"category":"MEDIUM",
			"recommendation":"MANUAL_REVIEW","computed_at":"2026-04-26T10:05:00Z"
		}`,
	})
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ application(id:"app_1") { id state applicantId riskAssessment { score category } } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	if !strings.Contains(got, `"id":"app_1"`) ||
		!strings.Contains(got, `"state":"manual_review"`) ||
		!strings.Contains(got, `"category":"MEDIUM"`) {
		t.Fatalf("unexpected data: %s", got)
	}
}

func TestQuery_AuditEvents(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/audit-events": `{"items":[
			{"id":"ev_1","tenant_id":"alpha","actor_type":"user","actor_id":"u_1",
			 "action":"application.approved","subject_type":"application","subject_id":"app_1",
			 "occurred_at":"2026-04-26T10:00:00Z","data":{"score":0.42}}
		]}`,
	})
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ auditEvents(filter:{limit:10}) { id action subjectId } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	if !strings.Contains(got, `"id":"ev_1"`) ||
		!strings.Contains(got, `"action":"application.approved"`) ||
		!strings.Contains(got, `"subjectId":"app_1"`) {
		t.Fatalf("unexpected data: %s", got)
	}
}

func TestMutation_SuspendTenant_RequiresPlatformAdmin(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/tenants/alpha/suspend": `{"id":"alpha","name":"Alpha","bik":"044525593","inn":"7728168971","status":"suspended","deployment_mode":"saas"}`,
	})
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()

	// 1) bank.admin → должно отклонить
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { suspendTenant(input:{tenantId:"alpha", reason:"x"}) { id status } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) == 0 {
		t.Fatalf("expected error for bank.admin trying suspendTenant; got data=%v", res.Data)
	}

	// 2) platform.admin → должно пройти
	res = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { suspendTenant(input:{tenantId:"alpha", reason:"x"}) { id status } }`,
		Context:       adminCtx(auth.RolePlatformAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"status":"suspended"`) {
		t.Fatalf("unexpected data: %s", data)
	}
}

// TestQuery_Applications_UsesAuthTenant — список заявок должен ходить
// в orchestrator с tenant_id из AuthContext.  Это исключает cross-
// tenant browsing для bank.admin'ов чужого тенанта.
func TestQuery_Applications_UsesAuthTenant(t *testing.T) {
	var capturedTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/applications") {
			capturedTenant = r.URL.Query().Get("tenant_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ applications { id } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	if capturedTenant != "alpha" {
		t.Fatalf("expected orchestrator query tenant_id=alpha (from auth), got %q", capturedTenant)
	}
}

// TestMutation_SuspendTenant_OperatorDenied — bank.compliance_officer
// тоже не может suspend'ить, только platform.admin.
func TestMutation_SuspendTenant_ComplianceOfficerDenied(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { suspendTenant(input:{tenantId:"alpha", reason:"x"}) { id } }`,
		Context:       adminCtx(auth.RoleBankComplianceOfficer),
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected forbidden error for bank.compliance_officer trying suspendTenant")
	}
}

func TestQuery_Unauthenticated(t *testing.T) {
	r := newTestResolver(t, "http://unused")
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ tenant { id } }`,
		Context:       context.Background(),
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected error for missing AuthContext")
	}
}

// ── ComplianceDashboard ───────────────────────────────────────────────
//
// Тесты используют мок-имплементации applicationsLister / uboCounter /
// auditCounter (определены в graph/dashboard.go) — это позволяет покрыть
// математику и обработку частичных сбоев без real HTTP.

type mockApps struct {
	apps []clients.Application
	err  error
}

func (m *mockApps) ListByDateRange(_ context.Context, _ string, _, _ time.Time) ([]clients.Application, error) {
	return m.apps, m.err
}

type mockUBO struct {
	total, matched int
	err            error
}

func (m *mockUBO) CountScreenings(_ context.Context, _ string, _, _ time.Time) (int, int, error) {
	return m.total, m.matched, m.err
}

type mockAudit struct {
	count int
	err   error
}

func (m *mockAudit) CountEvents(_ context.Context, _ string, _, _ time.Time, _ string) (int, error) {
	return m.count, m.err
}

func newDashboardResolver(apps *mockApps, ubo *mockUBO, aud *mockAudit) *Resolver {
	r := NewResolver(nil, nil, nil, nil, nil)
	r.WithDashboard(&Dashboard{Apps: apps, UBO: ubo, Audit: aud})
	return r
}

// ── Этапы 2-10 формы онбординга ─────────────────────────────────────

// TestQuery_LegalEntityProfile_ReturnsData — happy-path: orchestrator
// возвращает профиль, GraphQL раскладывает поля через reflection-резолвер.
func TestQuery_LegalEntityProfile_ReturnsData(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/legal-entity-profiles/by-application/app_1": `{
			"id":"prof_1","tenant_id":"alpha","application_id":"app_1","legal_entity_id":"le_1",
			"opf_code":"12200","registration_authority":"ФНС",
			"actual_same_as_legal":true,"postal_same_as_legal":true,
			"okved_main_v2":"62.01","okved_additional_v2":["63.11"],
			"contacts":{"phone":"+7","email":"a@b.c"},
			"created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"
		}`,
	})
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ legalEntityProfile(applicationId:"app_1") { id legalEntityId opfCode okvedMain contacts { email } } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	for _, want := range []string{
		`"id":"prof_1"`, `"legalEntityId":"le_1"`, `"opfCode":"12200"`,
		`"okvedMain":"62.01"`, `"email":"a@b.c"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in result, got: %s", want, got)
		}
	}
}

// TestQuery_LegalEntityProfile_NullOn404 — applicant ещё не дошёл до этапа 2.
// Ожидаем data.legalEntityProfile = null без ошибок.
func TestQuery_LegalEntityProfile_NullOn404(t *testing.T) {
	srv := stubServer(t, nil) // 404 на всё
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ legalEntityProfile(applicationId:"app_404") { id } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("expected no errors on 404, got: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"legalEntityProfile":null`) {
		t.Fatalf("expected legalEntityProfile=null, got: %s", data)
	}
}

// TestQuery_Representatives_UnwrapsItemsEnvelope — orchestrator возвращает
// {items:[...]}, GraphQL — массив. Также проверяем nested структуры.
func TestQuery_Representatives_UnwrapsItemsEnvelope(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/representatives/by-application/app_1": `{"items":[
			{"id":"rep_1","tenant_id":"alpha","application_id":"app_1","legal_entity_id":"le_1",
			 "last_name":"Иванов","first_name":"Иван","birth_date":"1980-01-01",
			 "id_document":{"doc_type":"passport_ru","number":"4500000001"},
			 "authority":{"position":"CEO","authority_basis":"charter"},
			 "is_primary":true,"is_signatory":true,
			 "created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"}
		]}`,
	})
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ representatives(applicationId:"app_1") { id lastName isPrimary idDocument { docType number } authority { position authorityBasis } } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	for _, want := range []string{
		`"id":"rep_1"`, `"lastName":"Иванов"`, `"isPrimary":true`,
		`"docType":"passport_ru"`, `"number":"4500000001"`,
		`"position":"CEO"`, `"authorityBasis":"charter"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in result, got: %s", want, got)
		}
	}
}

// TestQuery_Representatives_EmptyOn404 — на 404 возвращаем пустой массив,
// чтобы non-null контракт `[Representative!]!` не ломался.
func TestQuery_Representatives_EmptyOn404(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ representatives(applicationId:"app_404") { id } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("expected no errors on 404, got: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"representatives":[]`) {
		t.Fatalf("expected representatives=[], got: %s", data)
	}
}

// TestQuery_BankAccount_UsesAuthTenant — admin не может попросить чужой
// тенант: всегда подставляется tenant_id из AuthContext.
func TestQuery_BankAccount_UsesAuthTenant(t *testing.T) {
	var capturedTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/accounts/by-application/") {
			capturedTenant = r.URL.Query().Get("tenant_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"acc_1","tenant_id":"alpha","application_id":"app_1","legal_entity_id":"le_1",
				"currency":"RUB","account_type":"current",
				"agreements":{"agreement_acceptance":true,"agreement_accepted_at":"2026-04-26T10:00:00Z","dbo_agreement":true,"edo_agreement":true,"personal_data_consent":true,"signing_method":"sms_code"},
				"created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"
			}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ bankAccount(applicationId:"app_1") { id currency agreements { signingMethod } } }`,
		Context:       adminCtx(auth.RoleBankAdmin),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	if capturedTenant != "alpha" {
		t.Fatalf("expected tenant_id=alpha from auth, got %q", capturedTenant)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"signingMethod":"sms_code"`) {
		t.Fatalf("nested AccountAgreements not resolved correctly: %s", data)
	}
}

// TestComplianceDashboard_RequiresComplianceRole — bank.operator не должен
// получать доступ (даже если каким-то образом прошёл middleware).
func TestComplianceDashboard_RequiresComplianceRole(t *testing.T) {
	r := newDashboardResolver(&mockApps{}, &mockUBO{}, &mockAudit{})
	schema, _ := r.Schema()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u1", TenantID: "alpha", Role: auth.RoleBankOperator,
	})
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ complianceDashboard(tenantId:"alpha") { applicationsTotal } }`,
		Context:       ctx,
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected forbidden for bank.operator")
	}
	if !strings.Contains(res.Errors[0].Message, "compliance_officer") {
		t.Fatalf("expected role error, got: %v", res.Errors[0].Message)
	}
}

// TestComplianceDashboard_BlocksCrossTenant — auth.tenant=alpha, query
// tenant=beta → отказ (даже для bank.admin).
func TestComplianceDashboard_BlocksCrossTenant(t *testing.T) {
	r := newDashboardResolver(&mockApps{}, &mockUBO{}, &mockAudit{})
	schema, _ := r.Schema()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u1", TenantID: "alpha", Role: auth.RoleBankComplianceOfficer,
	})
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ complianceDashboard(tenantId:"beta") { applicationsTotal } }`,
		Context:       ctx,
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected cross-tenant forbidden")
	}
	if !strings.Contains(res.Errors[0].Message, "cross-tenant") {
		t.Fatalf("expected cross-tenant error, got: %v", res.Errors[0].Message)
	}

	// platform.admin должен пройти (override).
	ctx = auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u_p", TenantID: "alpha", Role: auth.RolePlatformAdmin,
	})
	res = graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ complianceDashboard(tenantId:"beta") { applicationsTotal } }`,
		Context:       ctx,
	})
	if len(res.Errors) > 0 {
		t.Fatalf("platform.admin should be allowed cross-tenant, got: %v", res.Errors)
	}
}

// TestComplianceDashboard_AggregatesCorrectly — мок upstream возвращает
// 10 заявок (5 auto_approved, 2 manual_review, 2 declined, 1 approved_with_edd);
// проверяем automation_rate=0.5, total=10, decisionsWithEDD=1.
func TestComplianceDashboard_AggregatesCorrectly(t *testing.T) {
	now := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	mk := func(state string, dt time.Duration) clients.Application {
		return clients.Application{
			ID: state, TenantID: "alpha", State: state,
			CreatedAt: now, UpdatedAt: now.Add(dt),
		}
	}
	apps := []clients.Application{
		mk("auto_approved", 30*time.Second),
		mk("auto_approved", 60*time.Second),
		mk("auto_approved", 90*time.Second),
		mk("auto_approved", 120*time.Second),
		mk("auto_approved", 100*time.Second),
		mk("manual_review", 0),
		mk("manual_review", 0),
		mk("declined", 200*time.Second),
		mk("declined", 200*time.Second),
		mk("approved_with_edd", 150*time.Second),
	}
	r := newDashboardResolver(
		&mockApps{apps: apps},
		&mockUBO{total: 7, matched: 1},
		&mockAudit{count: 42},
	)
	schema, _ := r.Schema()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u1", TenantID: "alpha", Role: auth.RoleBankComplianceOfficer,
	})
	res := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `{ complianceDashboard(tenantId:"alpha") {
			applicationsTotal decisionsAutoApproved decisionsManualReview
			decisionsDeclined decisionsWithEDD automationRate
			averageTimeToDecisionSec uboScreeningsTotal screeningsWithMatch
			auditEventsToday forgottenApplicantsCount
		} }`,
		Context: ctx,
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	for _, want := range []string{
		`"applicationsTotal":10`,
		`"decisionsAutoApproved":5`,
		`"decisionsManualReview":2`,
		`"decisionsDeclined":2`,
		`"decisionsWithEDD":1`,
		`"automationRate":0.5`,
		`"uboScreeningsTotal":7`,
		`"screeningsWithMatch":1`,
		`"auditEventsToday":42`,
		`"forgottenApplicantsCount":0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in result, got: %s", want, got)
		}
	}
	// avg time-to-decision: терминальные = 5 auto_approved (30+60+90+120+100=400)
	// + 2 declined (200+200=400) + 1 approved_with_edd (150) = 950с / 8 = 118.75с.
	// manual_review не считаем (не terminal).
	if !strings.Contains(got, `"averageTimeToDecisionSec":118.75`) {
		t.Errorf("expected averageTimeToDecisionSec=118.75, got: %s", got)
	}
}

// TestComplianceDashboard_HandlesPartialUpstreamFailure — orchestrator
// возвращает ошибку, но дашборд всё равно отдаётся: applicationsTotal=0,
// автоматические метрики тоже 0; UBO/audit поля заполнены.
func TestComplianceDashboard_HandlesPartialUpstreamFailure(t *testing.T) {
	r := newDashboardResolver(
		&mockApps{err: errors.New("orchestrator down")},
		&mockUBO{total: 5, matched: 2},
		&mockAudit{count: 11},
	)
	schema, _ := r.Schema()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		UserID: "u1", TenantID: "alpha", Role: auth.RoleBankComplianceOfficer,
	})
	res := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `{ complianceDashboard(tenantId:"alpha") {
			applicationsTotal automationRate uboScreeningsTotal auditEventsToday
		} }`,
		Context: ctx,
	})
	if len(res.Errors) > 0 {
		t.Fatalf("expected partial result, got errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	got := string(data)
	for _, want := range []string{
		`"applicationsTotal":0`,
		`"automationRate":0`,
		`"uboScreeningsTotal":5`,
		`"auditEventsToday":11`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in partial result, got: %s", want, got)
		}
	}
}

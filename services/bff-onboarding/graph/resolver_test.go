package graph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/graphql-go/graphql"

	"aibank/bff-onboarding/internal/auth"
	"aibank/bff-onboarding/internal/clients"
)

// stubServer возвращает httptest.Server, отвечающий разными JSON'ами на
// разные пути.  routes — мапа path-prefix → JSON-строка.
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
		clients.NewIdentityClient(srvURL),
		clients.NewDocumentClient(srvURL),
		clients.NewOrchestratorClient(srvURL),
		clients.NewRiskClient(srvURL),
	)
}

func authedCtx() context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		UserID:   "u_1",
		TenantID: "tnt_alpha",
		Role:     "client.applicant",
	})
}

func TestResolver_SchemaBuilds(t *testing.T) {
	r := newTestResolver(t, "http://unused.local")
	if _, err := r.Schema(); err != nil {
		t.Fatalf("Schema build: %v", err)
	}
}

func TestQuery_Me(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, err := r.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ me { userId tenantId role } }`,
		Context:       authedCtx(),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"userId":"u_1"`) ||
		!strings.Contains(string(data), `"tenantId":"tnt_alpha"`) {
		t.Fatalf("unexpected data: %s", data)
	}
}

func TestQuery_Me_Unauthenticated(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()

	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ me { userId } }`,
		Context:       context.Background(),
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected error for missing AuthContext")
	}
}

func TestQuery_Application(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/applications/app_1": `{
			"id":"app_1","tenant_id":"tnt_alpha","applicant_id":"u_1","legal_entity_type":"LLC",
			"channel":"web","state":"validating","product_codes":["current_rub"],
			"workflow_id":"wf_1","created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:01:00Z"
		}`,
		"/v1/documents":         `{"items":[]}`,
		"/v1/risk-assessments":  `{}`,
	})
	defer srv.Close()

	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()

	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ application(id:"app_1") { id state legalEntityType documents { id } } }`,
		Context:       authedCtx(),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"id":"app_1"`) ||
		!strings.Contains(string(data), `"state":"validating"`) {
		t.Fatalf("unexpected data: %s", data)
	}
}

// TestMutation_SubmitApplication_RequiresAuth — без AuthContext'а
// мутация должна вернуть ошибку unauthenticated.
func TestMutation_SubmitApplication_RequiresAuth(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `mutation {
			submitApplication(input:{
				legalEntityType:"LLC", channel:"web", productCodes:["x"]
			}) { id }
		}`,
		Context: context.Background(),
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected unauthenticated error")
	}
}

// TestMutation_SubmitApplication_UsesAuthTenant — проверяем, что
// resolver формирует body запроса в orchestrator с tenant_id из
// AuthContext, а не из аргументов мутации (защита от cross-tenant).
func TestMutation_SubmitApplication_UsesAuthTenant(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/applications":
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"application_id":"app_new","workflow_id":"wf_new"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/applications/app_new"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"app_new","tenant_id":"tnt_alpha","applicant_id":"u_1",
				"legal_entity_type":"LLC","channel":"web","state":"draft",
				"product_codes":["current_rub"],"workflow_id":"wf_new",
				"created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:00:00Z"
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema: schema,
		RequestString: `mutation {
			submitApplication(input:{
				legalEntityType:"LLC", channel:"web",
				productCodes:["current_rub"]
			}) { id tenantId }
		}`,
		Context: authedCtx(),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	// tenant_id в body запроса к orchestrator'у должен быть из claim'а.
	if !strings.Contains(capturedBody, `"tenant_id":"tnt_alpha"`) {
		t.Fatalf("orchestrator body must carry tenant_id from auth, got %s", capturedBody)
	}
	if !strings.Contains(capturedBody, `"applicant_id":"u_1"`) {
		t.Fatalf("orchestrator body must carry applicant_id from auth.UserID, got %s", capturedBody)
	}
}

// TestQuery_Application_RequiresAuth — /graphql Query.application без
// AuthContext'а должен фейлиться, защита от анонимных запросов.
func TestQuery_Application_RequiresAuth(t *testing.T) {
	srv := stubServer(t, nil)
	defer srv.Close()
	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()
	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `{ application(id:"app_1") { id } }`,
		Context:       context.Background(),
	})
	if len(res.Errors) == 0 {
		t.Fatal("expected unauthenticated error for Query.application")
	}
}

// TestQuery_Application_UsesAuthTenant — резолвер должен запрашивать
// orchestrator с tenant_id из AuthContext, а не из query-args.  Это
// исключает cross-tenant lookup даже если клиент попытается передать
// чужой ID.
func TestQuery_Application_UsesAuthTenant(t *testing.T) {
	var capturedTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/applications/") {
			capturedTenant = r.URL.Query().Get("tenant_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"app_1","tenant_id":"tnt_alpha","applicant_id":"u_1",
				"legal_entity_type":"LLC","channel":"web","state":"draft",
				"product_codes":["x"],"workflow_id":"wf",
				"created_at":"2026-04-26T10:00:00Z","updated_at":"2026-04-26T10:00:00Z"
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
		RequestString: `{ application(id:"app_1") { id } }`,
		Context:       authedCtx(),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	if capturedTenant != "tnt_alpha" {
		t.Fatalf("expected orchestrator query tenant_id=tnt_alpha (from auth), got %q", capturedTenant)
	}
}

func TestMutation_SendDocumentsUploadedSignal(t *testing.T) {
	srv := stubServer(t, map[string]string{
		"/v1/documents": `{"items":[
			{"id":"doc_1","type":"PASSPORT","application_id":"app_1","filename":"p.pdf","state":"uploaded","uploaded_at":"2026-04-26T10:00:00Z"}
		]}`,
		"/v1/applications/app_1/signals/documents-uploaded": `{"status":"accepted"}`,
	})
	defer srv.Close()

	r := newTestResolver(t, srv.URL)
	schema, _ := r.Schema()

	res := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: `mutation { sendDocumentsUploadedSignal(applicationId:"app_1") }`,
		Context:       authedCtx(),
	})
	if len(res.Errors) > 0 {
		t.Fatalf("graphql errors: %+v", res.Errors)
	}
	data, _ := json.Marshal(res.Data)
	if !strings.Contains(string(data), `"sendDocumentsUploadedSignal":true`) {
		t.Fatalf("unexpected data: %s", data)
	}
}

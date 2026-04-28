// Package graph — GraphQL-резолверы bff-onboarding.
//
// Текущий runtime — github.com/graphql-go/graphql (hand-rolled), это
// позволяет собирать и тестировать BFF без шага codegen.  Контракт
// описан в graph/schema.graphqls; gqlgen.yml готов к
// `go run github.com/99designs/gqlgen generate` (см. ADR-0003).
//
// Резолвер — тонкий fan-out:
//   - читает AuthContext из context.Context (положен JWT-middleware'ом);
//   - параллельно ходит в downstream-сервисы через internal/clients;
//   - возвращает доменные типы из internal/model.
package graph

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"aibank/bff-onboarding/internal/auth"
	"aibank/bff-onboarding/internal/clients"
	"aibank/bff-onboarding/internal/model"
)

// Resolver агрегирует клиенты ко всем downstream-сервисам.
type Resolver struct {
	Tenant       *clients.TenantClient
	Identity     *clients.IdentityClient
	Document     *clients.DocumentClient
	Orchestrator *clients.OrchestratorClient
	Risk         *clients.RiskClient
}

// NewResolver — конструктор; принимает уже сконфигурированные клиенты,
// чтобы упростить unit-тесты с httptest.Server.
func NewResolver(
	tenant *clients.TenantClient,
	identity *clients.IdentityClient,
	document *clients.DocumentClient,
	orchestrator *clients.OrchestratorClient,
	risk *clients.RiskClient,
) *Resolver {
	return &Resolver{
		Tenant:       tenant,
		Identity:     identity,
		Document:     document,
		Orchestrator: orchestrator,
		Risk:         risk,
	}
}

// Schema собирает graphql.Schema с типами из schema.graphqls.
//
// При изменениях schema.graphqls правки нужны и здесь, пока не сделан
// переход на gqlgen-codegen.
func (r *Resolver) Schema() (graphql.Schema, error) {
	timeScalar := graphql.DateTime
	jsonScalar := graphql.NewScalar(graphql.ScalarConfig{
		Name:         "JSON",
		Description:  "Произвольный JSON-объект.",
		Serialize:    func(v interface{}) interface{} { return v },
		ParseValue:   func(v interface{}) interface{} { return v },
		ParseLiteral: func(v ast.Value) interface{} { return v.GetValue() },
	})

	stateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name:   "ApplicationState",
		Values: stateEnumValues(),
	})
	docTypeEnum := graphql.NewEnum(graphql.EnumConfig{
		Name:   "DocumentType",
		Values: docTypeEnumValues(),
	})
	riskCategoryEnum := graphql.NewEnum(graphql.EnumConfig{
		Name:   "RiskCategory",
		Values: riskCategoryEnumValues(),
	})
	decisionKindEnum := graphql.NewEnum(graphql.EnumConfig{
		Name:   "DecisionKind",
		Values: decisionKindEnumValues(),
	})

	meType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Me",
		Fields: graphql.Fields{
			"userId":   &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"tenantId": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"role":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"email":    &graphql.Field{Type: graphql.String},
		},
	})

	personType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Person",
		Fields: graphql.Fields{
			"id":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"fullName": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"inn":      &graphql.Field{Type: graphql.String},
			"phone":    &graphql.Field{Type: graphql.String},
			"email":    &graphql.Field{Type: graphql.String},
		},
	})

	documentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Document",
		Fields: graphql.Fields{
			"id":            &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"type":          &graphql.Field{Type: graphql.NewNonNull(docTypeEnum)},
			"applicationId": &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"filename":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"state":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"uploadedAt":    &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
		},
	})

	riskAssessmentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RiskAssessment",
		Fields: graphql.Fields{
			"id":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"applicationId":  &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"score":          &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"category":       &graphql.Field{Type: graphql.NewNonNull(riskCategoryEnum)},
			"recommendation": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"computedAt":     &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"factors":        &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
		},
	})

	discrepancyType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Discrepancy",
		Fields: graphql.Fields{
			"field":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"documentId": &graphql.Field{Type: graphql.ID},
			"expected":   &graphql.Field{Type: graphql.String},
			"actual":     &graphql.Field{Type: graphql.String},
			"severity":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	decisionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Decision",
		Fields: graphql.Fields{
			"id":            &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"applicationId": &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"decision":      &graphql.Field{Type: graphql.NewNonNull(decisionKindEnum)},
			"reasoning":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"decidedAt":     &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
		},
	})

	tenantType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Tenant",
		Fields: graphql.Fields{
			"id":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"name":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"bik":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"inn":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"status":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"deploymentMode": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	_ = tenantType // экспонируется через Application в будущей итерации

	applicationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Application",
		Fields: graphql.Fields{
			"id":              &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"tenantId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"state":           &graphql.Field{Type: graphql.NewNonNull(stateEnum)},
			"legalEntityType": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"channel":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"productCodes":    &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String)))},
			"createdAt":       &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"updatedAt":       &graphql.Field{Type: graphql.NewNonNull(timeScalar)},

			"applicant": &graphql.Field{
				Type:    personType,
				Resolve: r.resolveApplicant,
			},
			"documents": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(documentType))),
				Resolve: r.resolveDocuments,
			},
			"riskAssessment": &graphql.Field{
				Type:    riskAssessmentType,
				Resolve: r.resolveRiskAssessment,
			},
			"decision": &graphql.Field{
				Type:    decisionType,
				Resolve: nil, // TODO: decision-service-клиент в отдельной итерации.
			},
			"discrepancies": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(discrepancyType))),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) { return []model.Discrepancy{}, nil },
			},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"me":              &graphql.Field{Type: graphql.NewNonNull(meType), Resolve: r.resolveMe},
			"application":     &graphql.Field{Type: applicationType, Args: graphql.FieldConfigArgument{"id": {Type: graphql.NewNonNull(graphql.ID)}}, Resolve: r.resolveApplication},
			"myApplications":  &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(applicationType))), Resolve: r.resolveMyApplications},
		},
	})

	submitInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SubmitApplicationInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"legalEntityType": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"channel":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"productCodes":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String)))},
		},
	})
	uploadInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UploadDocumentInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"type":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(docTypeEnum)},
			"filename":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"contentBase64": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"submitApplication": &graphql.Field{
				Type:    graphql.NewNonNull(applicationType),
				Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(submitInput)}},
				Resolve: r.resolveSubmitApplication,
			},
			"uploadDocument": &graphql.Field{
				Type:    graphql.NewNonNull(documentType),
				Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(uploadInput)}},
				Resolve: r.resolveUploadDocument,
			},
			"sendDocumentsUploadedSignal": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.Boolean),
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveSendDocumentsUploadedSignal,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
}

// ── helpers ──────────────────────────────────────────────────────────

// stateEnumValues — enum-карта, в которой Value хранит typed-значение
// model.ApplicationState (а не голый string).  graphql-go использует
// reflect.DeepEqual при сериализации, поэтому Value должно быть строго
// того же type, что возвращают резолверы (Application.State).
func stateEnumValues() graphql.EnumValueConfigMap {
	states := []model.ApplicationState{
		model.StateDraft, model.StateIdentifying, model.StateCollectingDocuments, model.StateValidating,
		model.StateWaitingForClient, model.StateRiskAssessing, model.StateAutoApproved, model.StateManualReview,
		model.StateApproved, model.StateApprovedWithEDD, model.StateRequiresMoreInfo, model.StateOpeningAccount,
		model.StateAccountOpened, model.StateDeclined, model.StateAbandoned,
	}
	out := make(graphql.EnumValueConfigMap, len(states))
	for _, s := range states {
		out[string(s)] = &graphql.EnumValueConfig{Value: s}
	}
	return out
}

func docTypeEnumValues() graphql.EnumValueConfigMap {
	types := []model.DocumentType{
		model.DocPassport, model.DocCharter, model.DocProtocol, model.DocAgreement,
		model.DocEgrulExtract, model.DocPowerOfAttorney, model.DocAccountingReport, model.DocOther,
	}
	out := make(graphql.EnumValueConfigMap, len(types))
	for _, dt := range types {
		out[string(dt)] = &graphql.EnumValueConfig{Value: dt}
	}
	return out
}

func riskCategoryEnumValues() graphql.EnumValueConfigMap {
	cats := []model.RiskCategory{model.RiskLow, model.RiskMedium, model.RiskHigh}
	out := make(graphql.EnumValueConfigMap, len(cats))
	for _, c := range cats {
		out[string(c)] = &graphql.EnumValueConfig{Value: c}
	}
	return out
}

func decisionKindEnumValues() graphql.EnumValueConfigMap {
	kinds := []model.DecisionKind{
		model.DecisionApproved, model.DecisionApprovedWithEDD, model.DecisionDeclined, model.DecisionEscalated,
	}
	out := make(graphql.EnumValueConfigMap, len(kinds))
	for _, k := range kinds {
		out[string(k)] = &graphql.EnumValueConfig{Value: k}
	}
	return out
}

// ErrUnauthenticated — нет AuthContext в request context.
var ErrUnauthenticated = errors.New("unauthenticated")

func authFrom(ctx context.Context) (*auth.AuthContext, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil {
		return nil, ErrUnauthenticated
	}
	return ac, nil
}

// parallel — простая реализация fan-out из резолвера.  Каждый f запускается
// в горутине; ошибки собираются.
func parallel(ctx context.Context, fns ...func(context.Context) error) error {
	var wg sync.WaitGroup
	errs := make([]error, len(fns))
	for i, f := range fns {
		wg.Add(1)
		go func(i int, f func(context.Context) error) {
			defer wg.Done()
			errs[i] = f(ctx)
		}(i, f)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// ── Query resolvers ──────────────────────────────────────────────────

func (r *Resolver) resolveMe(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	return clients.MeFromAuth(ac), nil
}

func (r *Resolver) resolveApplication(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	app, err := r.Orchestrator.GetApplication(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return app, nil
}

func (r *Resolver) resolveMyApplications(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	apps, err := r.Orchestrator.ListByApplicant(p.Context, ac.TenantID, ac.UserID)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

// ── Field resolvers ──────────────────────────────────────────────────

func (r *Resolver) resolveApplicant(p graphql.ResolveParams) (interface{}, error) {
	app, ok := p.Source.(*model.Application)
	if !ok {
		// non-pointer fallback
		if v, ok := p.Source.(model.Application); ok {
			app = &v
		} else {
			return nil, nil
		}
	}
	if app == nil || app.ApplicantID == "" {
		return nil, nil
	}
	// Минимальный shape — applicant_id + UserID из claims.  Полная
	// загрузка профиля через identity-service не нужна для UI клиента.
	// TODO: заменить на dataloader-batched lookup при добавлении полей.
	return &model.Person{ID: app.ApplicantID}, nil
}

func (r *Resolver) resolveDocuments(p graphql.ResolveParams) (interface{}, error) {
	app := applicationFromSource(p.Source)
	if app == nil {
		return []model.Document{}, nil
	}
	return r.Document.ListByApplication(p.Context, app.TenantID, app.ID)
}

func (r *Resolver) resolveRiskAssessment(p graphql.ResolveParams) (interface{}, error) {
	app := applicationFromSource(p.Source)
	if app == nil {
		return nil, nil
	}
	return r.Risk.GetByApplication(p.Context, app.TenantID, app.ID)
}

func applicationFromSource(src interface{}) *model.Application {
	switch v := src.(type) {
	case *model.Application:
		return v
	case model.Application:
		return &v
	}
	return nil
}

// ── Mutation resolvers ───────────────────────────────────────────────

func (r *Resolver) resolveSubmitApplication(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input is required")
	}
	productsRaw, _ := in["productCodes"].([]interface{})
	products := make([]string, 0, len(productsRaw))
	for _, v := range productsRaw {
		if s, ok := v.(string); ok {
			products = append(products, s)
		}
	}
	legalEntityType, _ := in["legalEntityType"].(string)
	channel, _ := in["channel"].(string)
	if legalEntityType == "" || channel == "" || len(products) == 0 {
		return nil, fmt.Errorf("legalEntityType, channel and productCodes are required")
	}

	app, err := r.Orchestrator.Submit(p.Context, clients.SubmitInput{
		TenantID:        ac.TenantID,
		ApplicantID:     ac.UserID,
		LegalEntityType: legalEntityType,
		Channel:         channel,
		ProductCodes:    products,
		// MVP-пороги; per-tenant конфиг подтягивается tenant-service'ом.
		RiskThresholds: map[string]any{"auto_approve_below": 0.3, "decline_above": 0.8},
	})
	if err != nil {
		return nil, err
	}
	return app, nil
}

func (r *Resolver) resolveUploadDocument(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input is required")
	}
	applicationID, _ := in["applicationId"].(string)
	docType, _ := in["type"].(string)
	filename, _ := in["filename"].(string)
	content, _ := in["contentBase64"].(string)
	if applicationID == "" || docType == "" || filename == "" || content == "" {
		return nil, fmt.Errorf("all fields of UploadDocumentInput are required")
	}
	doc, err := r.Document.Upload(p.Context, ac.TenantID, clients.UploadInput{
		ApplicationID: applicationID,
		Type:          docType,
		Filename:      filename,
		ContentBase64: content,
	})
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (r *Resolver) resolveSendDocumentsUploadedSignal(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return false, err
	}
	applicationID, _ := p.Args["applicationId"].(string)
	if applicationID == "" {
		return false, fmt.Errorf("applicationId is required")
	}
	// Подтягиваем актуальный список документов в orchestrator-сигнал, чтобы
	// он мог проверить полноту.  Параметры — минимально-валидные.
	docs, err := r.Document.ListByApplication(p.Context, ac.TenantID, applicationID)
	if err != nil {
		return false, err
	}
	refs := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		refs = append(refs, map[string]any{
			"document_id": d.ID,
			"type":        string(d.Type),
		})
	}
	if err := r.Orchestrator.SignalDocumentsUploaded(p.Context, ac.TenantID, applicationID, refs); err != nil {
		return false, err
	}
	return true, nil
}

// Package graph — GraphQL-резолверы bff-admin.
//
// graphql-go/graphql hand-rolled (без gqlgen, согласовано с bff-onboarding,
// см. ADR-0003).  Резолверы — тонкий fan-out с обязательной проверкой
// AuthContext.  Тонкая авторизация мутаций (suspendTenant → platform.admin) —
// здесь; широкая (admin-доступ) — в auth.Middleware.  complianceDashboard:
// см. dashboard.go.
package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"aibank/bff-admin/internal/auth"
	"aibank/bff-admin/internal/clients"
)

// Resolver агрегирует клиентов downstream-сервисов.
type Resolver struct {
	Tenant       *clients.TenantClient
	Audit        *clients.AuditClient
	Orchestrator *clients.OrchestratorClient
	Risk         *clients.RiskClient
	Identity     *clients.IdentityClient

	// dashboard — зависимости complianceDashboard (см. dashboard.go).
	dashboard *Dashboard
}

// NewResolver — конструктор; dashboard собирается из переданных клиентов
// (тесты могут переопределить через WithDashboard).
func NewResolver(
	tenant *clients.TenantClient,
	audit *clients.AuditClient,
	orchestrator *clients.OrchestratorClient,
	risk *clients.RiskClient,
	identity *clients.IdentityClient,
) *Resolver {
	r := &Resolver{
		Tenant: tenant, Audit: audit, Orchestrator: orchestrator,
		Risk: risk, Identity: identity,
	}
	r.dashboard = &Dashboard{
		Apps:  orchestrator,
		UBO:   nil, // soft endpoint; main.go подключит при наличии URL
		Audit: NewAuditCounter(audit),
	}
	return r
}

// WithDashboard — переопределяет зависимости complianceDashboard (для тестов и main.go).
func (r *Resolver) WithDashboard(d *Dashboard) *Resolver { r.dashboard = d; return r }

// ErrUnauthenticated — нет AuthContext (не должно случиться при корректной middleware-цепочке).
var ErrUnauthenticated = errors.New("unauthenticated")

// ErrNotPlatformAdmin — мутация требует platform.admin.
var ErrNotPlatformAdmin = errors.New("forbidden: platform.admin required")

func authFrom(ctx context.Context) (*auth.AuthContext, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil {
		return nil, ErrUnauthenticated
	}
	return ac, nil
}

// Schema собирает graphql.Schema; экспортируется для main.go.
//
// При правке schema.graphqls — отсюда тоже.
func (r *Resolver) Schema() (graphql.Schema, error) {
	timeScalar := graphql.DateTime
	jsonScalar := graphql.NewScalar(graphql.ScalarConfig{
		Name:         "JSON",
		Serialize:    func(v interface{}) interface{} { return v },
		ParseValue:   func(v interface{}) interface{} { return v },
		ParseLiteral: func(v ast.Value) interface{} { return v.GetValue() },
	})

	stateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "ApplicationState",
		Values: graphql.EnumValueConfigMap{
			"draft":                {Value: "draft"},
			"identifying":          {Value: "identifying"},
			"collecting_documents": {Value: "collecting_documents"},
			"validating":           {Value: "validating"},
			"waiting_for_client":   {Value: "waiting_for_client"},
			"risk_assessing":       {Value: "risk_assessing"},
			"auto_approved":        {Value: "auto_approved"},
			"manual_review":        {Value: "manual_review"},
			"approved":             {Value: "approved"},
			"approved_with_edd":    {Value: "approved_with_edd"},
			"requires_more_info":   {Value: "requires_more_info"},
			"opening_account":      {Value: "opening_account"},
			"account_opened":       {Value: "account_opened"},
			"declined":             {Value: "declined"},
			"abandoned":            {Value: "abandoned"},
		},
	})
	decisionKindEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "DecisionKind",
		Values: graphql.EnumValueConfigMap{
			"APPROVED":          {Value: "APPROVED"},
			"APPROVED_WITH_EDD": {Value: "APPROVED_WITH_EDD"},
			"DECLINED":          {Value: "DECLINED"},
			"ESCALATED":         {Value: "ESCALATED"},
		},
	})
	riskCategoryEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "RiskCategory",
		Values: graphql.EnumValueConfigMap{
			"LOW":    {Value: "LOW"},
			"MEDIUM": {Value: "MEDIUM"},
			"HIGH":   {Value: "HIGH"},
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

	riskType := graphql.NewObject(graphql.ObjectConfig{
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

	applicationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Application",
		Fields: graphql.Fields{
			"id":              &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"tenantId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"applicantId":     &graphql.Field{Type: graphql.ID},
			"state":           &graphql.Field{Type: graphql.NewNonNull(stateEnum)},
			"legalEntityType": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"channel":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"productCodes":    &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String)))},
			"createdAt":       &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"updatedAt":       &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"riskAssessment": &graphql.Field{
				Type:    riskType,
				Resolve: r.resolveRisk,
			},
			"decision": &graphql.Field{
				Type:    decisionType,
				Resolve: nil,
			},
		},
	})

	auditEventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AuditEvent",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"tenantId":    &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"actorType":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"actorId":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"action":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"subjectType": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"subjectId":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"occurredAt":  &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"data":        &graphql.Field{Type: jsonScalar},
		},
	})

	tenantConfigType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TenantConfig",
		Fields: graphql.Fields{
			"tenantId":      &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"schemaVersion": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"rawConfig":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":     &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
		},
	})

	complianceDashboardType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ComplianceDashboard",
		Fields: graphql.Fields{
			"tenantId":                 &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"from":                     &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"to":                       &graphql.Field{Type: graphql.NewNonNull(timeScalar)},
			"applicationsTotal":        &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"applicationsByState":      &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
			"decisionsAutoApproved":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"decisionsManualReview":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"decisionsDeclined":        &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"decisionsWithEDD":         &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"automationRate":           &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"averageTimeToDecisionSec": &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"uboScreeningsTotal":       &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"screeningsWithMatch":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"auditEventsToday":         &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"forgottenApplicantsCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	applicationsFilter := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ApplicationsFilter",
		Fields: graphql.InputObjectConfigFieldMap{
			"state":           &graphql.InputObjectFieldConfig{Type: stateEnum},
			"legalEntityType": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"limit":           &graphql.InputObjectFieldConfig{Type: graphql.Int},
		},
	})
	auditFilter := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "AuditFilter",
		Fields: graphql.InputObjectConfigFieldMap{
			"actor":  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"action": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"limit":  &graphql.InputObjectFieldConfig{Type: graphql.Int},
		},
	})
	updateDecisionInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpdateApplicationDecisionInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"decision":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(decisionKindEnum)},
			"reasoning":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	suspendInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SuspendTenantInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"tenantId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"reason":   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	transitionInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "TransitionApplicationStateInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"newState":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"reason":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"tenant": &graphql.Field{Type: tenantType, Resolve: r.resolveTenant},
			"applications": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(applicationType))),
				Args:    graphql.FieldConfigArgument{"filter": {Type: applicationsFilter}},
				Resolve: r.resolveApplications,
			},
			"application": &graphql.Field{
				Type:    applicationType,
				Args:    graphql.FieldConfigArgument{"id": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveApplication,
			},
			"auditEvents": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(auditEventType))),
				Args:    graphql.FieldConfigArgument{"filter": {Type: auditFilter}},
				Resolve: r.resolveAuditEvents,
			},
			"tenantConfig": &graphql.Field{Type: tenantConfigType, Resolve: r.resolveTenantConfig},
			"complianceDashboard": &graphql.Field{
				Type: graphql.NewNonNull(complianceDashboardType),
				Args: graphql.FieldConfigArgument{
					"tenantId": {Type: graphql.NewNonNull(graphql.String)},
					"from":     {Type: timeScalar},
					"to":       {Type: timeScalar},
				},
				Resolve: r.resolveComplianceDashboard,
			},
		},
	})

	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"updateApplicationDecision": &graphql.Field{
				Type:    graphql.NewNonNull(decisionType),
				Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(updateDecisionInput)}},
				Resolve: r.resolveUpdateDecision,
			},
			"suspendTenant": &graphql.Field{
				Type:    graphql.NewNonNull(tenantType),
				Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(suspendInput)}},
				Resolve: r.resolveSuspendTenant,
			},
			"transitionApplicationState": &graphql.Field{
				Type:    graphql.NewNonNull(applicationType),
				Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(transitionInput)}},
				Resolve: r.resolveTransitionApplicationState,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{Query: queryType, Mutation: mutationType})
}

// ── Query resolvers ──────────────────────────────────────────────────

func (r *Resolver) resolveTenant(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	if ac.TenantID == "" {
		return nil, nil
	}
	return r.Tenant.Get(p.Context, ac.TenantID)
}

func (r *Resolver) resolveApplications(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	f := clients.ListFilter{}
	if in, ok := p.Args["filter"].(map[string]interface{}); ok {
		if s, ok := in["state"].(string); ok {
			f.State = s
		}
		if s, ok := in["legalEntityType"].(string); ok {
			f.LegalEntityType = s
		}
		if l, ok := in["limit"].(int); ok {
			f.Limit = l
		}
	}
	return r.Orchestrator.List(p.Context, ac.TenantID, f)
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
	app, err := r.Orchestrator.Get(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return app, nil
}

func (r *Resolver) resolveAuditEvents(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	f := clients.AuditFilter{Limit: 50}
	if in, ok := p.Args["filter"].(map[string]interface{}); ok {
		if s, ok := in["actor"].(string); ok {
			f.Actor = s
		}
		if s, ok := in["action"].(string); ok {
			f.Action = s
		}
		if l, ok := in["limit"].(int); ok && l > 0 {
			f.Limit = l
		}
	}
	events, err := r.Audit.List(p.Context, ac.TenantID, f)
	if err != nil {
		return nil, err
	}
	// graphql-go ожидает map для JSON-полей; преобразуем data → map[string]any.
	out := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		var data interface{}
		if len(e.Data) > 0 {
			_ = json.Unmarshal(e.Data, &data)
		}
		out = append(out, map[string]interface{}{
			"id":          e.ID,
			"tenantId":    e.TenantID,
			"actorType":   e.ActorType,
			"actorId":     e.ActorID,
			"action":      e.Action,
			"subjectType": e.SubjectType,
			"subjectId":   e.SubjectID,
			"occurredAt":  e.OccurredAt,
			"data":        data,
		})
	}
	return out, nil
}

func (r *Resolver) resolveTenantConfig(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	cfg, err := r.Tenant.GetConfig(p.Context, ac.TenantID)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return map[string]interface{}{
		"tenantId":      cfg.TenantID,
		"rawConfig":     cfg.RawConfig,
		"schemaVersion": cfg.SchemaVersion,
		"updatedAt":     cfg.UpdatedAt,
	}, nil
}

func (r *Resolver) resolveRisk(p graphql.ResolveParams) (interface{}, error) {
	app, ok := p.Source.(*clients.Application)
	if !ok {
		if v, ok := p.Source.(clients.Application); ok {
			app = &v
		}
	}
	if app == nil {
		return nil, nil
	}
	ra, err := r.Risk.GetByApplication(p.Context, app.TenantID, app.ID)
	if err != nil || ra == nil {
		return nil, err
	}
	// Поля factors — пробросим их как []interface{} через JSON parse.
	factors := make([]interface{}, 0, len(ra.Factors))
	for _, f := range ra.Factors {
		var v interface{}
		if err := json.Unmarshal(f, &v); err == nil {
			factors = append(factors, v)
		}
	}
	return map[string]interface{}{
		"id":             ra.ID,
		"applicationId":  ra.ApplicationID,
		"score":          ra.Score,
		"category":       ra.Category,
		"recommendation": ra.Recommendation,
		"computedAt":     ra.ComputedAt,
		"factors":        factors,
	}, nil
}

// ── Mutation resolvers ───────────────────────────────────────────────

// resolveTransitionApplicationState — Mutation.transitionApplicationState.
// Прокси к orchestrator POST /v1/applications/{id}/transitions. Не делает
// дополнительной авторизации — bff-admin уже требует bank.* / platform.admin
// в middleware. Audit-trail формируется на стороне orchestrator.
func (r *Resolver) resolveTransitionApplicationState(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input required")
	}
	applicationID, _ := in["applicationId"].(string)
	newState, _ := in["newState"].(string)
	reason, _ := in["reason"].(string)
	if applicationID == "" || newState == "" {
		return nil, fmt.Errorf("applicationId and newState are required")
	}
	return r.Orchestrator.TransitionState(p.Context, ac.TenantID, applicationID, newState, reason)
}

func (r *Resolver) resolveUpdateDecision(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input required")
	}
	applicationID, _ := in["applicationId"].(string)
	decision, _ := in["decision"].(string)
	reasoning, _ := in["reasoning"].(string)
	if applicationID == "" || decision == "" || reasoning == "" {
		return nil, fmt.Errorf("applicationId, decision and reasoning are required")
	}
	return r.Orchestrator.UpdateDecision(p.Context, ac.TenantID, applicationID, decision, reasoning)
}

func (r *Resolver) resolveSuspendTenant(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	if !auth.IsPlatformAdmin(ac.Role) {
		return nil, ErrNotPlatformAdmin
	}
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input required")
	}
	tenantID, _ := in["tenantId"].(string)
	reason, _ := in["reason"].(string)
	if tenantID == "" || reason == "" {
		return nil, fmt.Errorf("tenantId and reason are required")
	}
	return r.Tenant.Suspend(p.Context, tenantID, reason)
}

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
	"reflect"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"aibank/bff-admin/internal/auth"
	"aibank/bff-admin/internal/clients"
)

// fieldResolver — generic resolver, который читает значение Go-struct field
// по имени (через reflect). Используется для типов этапов 2-10 формы
// онбординга — каждое поле имеет camelCase GraphQL-имя и snake_case JSON-tag,
// поэтому graphql-go не может resolve их default'ом (он ищет именно
// camelCase / нижний регистр в JSON-tag). Helper решает это однажды для
// всех типов profile/activity/representative/UBO/screening/monitoring/account.
//
// Принимает указатель/значение/nil и nested-структуры тоже резолвит.
func fieldResolver(goFieldName string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		if p.Source == nil {
			return nil, nil
		}
		v := reflect.ValueOf(p.Source)
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil, nil
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil, nil
		}
		f := v.FieldByName(goFieldName)
		if !f.IsValid() {
			return nil, nil
		}
		// nil pointer-поля → nil (а не *T(nil)).
		if f.Kind() == reflect.Ptr && f.IsNil() {
			return nil, nil
		}
		return f.Interface(), nil
	}
}

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

	// ── Этапы 2-10 формы онбординга — output types ────────────────────

	structuredAddressType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StructuredAddress",
		Fields: graphql.Fields{
			"countryCode": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("CountryCode")},
			"postalCode":  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("PostalCode")},
			"regionCode":  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("RegionCode")},
			"regionName":  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("RegionName")},
			"city":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("City")},
			"street":      &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Street")},
			"building":    &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Building")},
			"office":      &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Office")},
			"fiasId":      &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("FIASID")},
		},
	})

	moneyAmountType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MoneyAmount",
		Fields: graphql.Fields{
			"amount":   &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("Amount")},
			"currency": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Currency")},
		},
	})

	contactInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ContactInfo",
		Fields: graphql.Fields{
			"phone":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Phone")},
			"email":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Email")},
			"website": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Website")},
		},
	})

	licenseInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LicenseInfo",
		Fields: graphql.Fields{
			"number":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Number")},
			"issueDate":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("IssueDate")},
			"expiryDate":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ExpiryDate")},
			"issuer":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Issuer")},
			"activityType": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("ActivityType")},
		},
	})

	sroMembershipType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SROMembership",
		Fields: graphql.Fields{
			"name":      &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Name")},
			"regNumber": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("RegNumber")},
			"joinDate":  &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("JoinDate")},
		},
	})

	legalEntityProfileType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LegalEntityProfile",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"legalEntityId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("LegalEntityID")},
			"opfCode":               &graphql.Field{Type: graphql.String, Resolve: fieldResolver("OPFCode")},
			"registrationAuthority": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("RegistrationAuthority")},
			"authorizedCapital":     &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("AuthorizedCapital")},
			"legalAddress":          &graphql.Field{Type: structuredAddressType, Resolve: fieldResolver("LegalAddress")},
			"actualAddress":         &graphql.Field{Type: structuredAddressType, Resolve: fieldResolver("ActualAddress")},
			"actualSameAsLegal":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("ActualSameAsLegal")},
			"postalAddress":         &graphql.Field{Type: structuredAddressType, Resolve: fieldResolver("PostalAddress")},
			"postalSameAsLegal":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("PostalSameAsLegal")},
			"okvedMain":             &graphql.Field{Type: graphql.String, Resolve: fieldResolver("OKVEDMain")},
			"okvedAdditional":       &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("OKVEDAdditional")},
			"licenses":              &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(licenseInfoType)), Resolve: fieldResolver("Licenses")},
			"sroMembership":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(sroMembershipType)), Resolve: fieldResolver("SROMembership")},
			"contacts":              &graphql.Field{Type: contactInfoType, Resolve: fieldResolver("Contacts")},
			"employeesCount":        &graphql.Field{Type: graphql.Int, Resolve: fieldResolver("EmployeesCount")},
			"revenueLastYear":       &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("RevenueLastYear")},
			"taxRegime":             &graphql.Field{Type: graphql.String, Resolve: fieldResolver("TaxRegime")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	counterpartyType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Counterparty",
		Fields: graphql.Fields{
			"name":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Name")},
			"inn":              &graphql.Field{Type: graphql.String, Resolve: fieldResolver("INN")},
			"country":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Country")},
			"sharePercent":     &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("SharePercent")},
			"relationshipType": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("RelationshipType")},
		},
	})

	operationalModelType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OperationalModel",
		Fields: graphql.Fields{
			"geography":               &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("Geography")},
			"monthlyTurnoverPlanned":  &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("MonthlyTurnoverPlanned")},
			"annualTurnoverPlanned":   &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("AnnualTurnoverPlanned")},
			"cashSharePercent":        &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("CashSharePercent")},
			"foreignEconomicActivity": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("ForeignEconomicActivity")},
			"foreignCountries":        &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("ForeignCountries")},
			"currencyOperations":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("CurrencyOperations")},
		},
	})

	fundsSourceType := graphql.NewObject(graphql.ObjectConfig{
		Name: "FundsSource",
		Fields: graphql.Fields{
			"category":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Category")},
			"description": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Description")},
		},
	})

	applicationActivityType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ApplicationActivity",
		Fields: graphql.Fields{
			"id":                  &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":            &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"businessDescription": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("BusinessDescription")},
			"businessCategory":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("BusinessCategory")},
			"topSuppliers":        &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(counterpartyType)), Resolve: fieldResolver("TopSuppliers")},
			"topBuyers":           &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(counterpartyType)), Resolve: fieldResolver("TopBuyers")},
			"operationalModel":    &graphql.Field{Type: operationalModelType, Resolve: fieldResolver("OperationalModel")},
			"fundsSource":         &graphql.Field{Type: graphql.NewNonNull(fundsSourceType), Resolve: fieldResolver("FundsSource")},
			"createdAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	idDocumentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IDDocument",
		Fields: graphql.Fields{
			"docType":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("DocType")},
			"series":         &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Series")},
			"number":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Number")},
			"issueDate":      &graphql.Field{Type: graphql.String, Resolve: fieldResolver("IssueDate")},
			"expiryDate":     &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ExpiryDate")},
			"issuedBy":       &graphql.Field{Type: graphql.String, Resolve: fieldResolver("IssuedBy")},
			"departmentCode": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("DepartmentCode")},
		},
	})

	authorityInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AuthorityInfo",
		Fields: graphql.Fields{
			"position":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Position")},
			"authorityBasis":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("AuthorityBasis")},
			"authorityDocNumber":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("AuthorityDocNumber")},
			"authorityDocDate":     &graphql.Field{Type: graphql.String, Resolve: fieldResolver("AuthorityDocDate")},
			"signatureSampleDocId": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("SignatureSampleDocID")},
		},
	})

	foreignerInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ForeignerInfo",
		Fields: graphql.Fields{
			"migrationCardNumber":    &graphql.Field{Type: graphql.String, Resolve: fieldResolver("MigrationCardNumber")},
			"migrationCardIssuedAt":  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("MigrationCardIssuedAt")},
			"migrationCardExpiresAt": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("MigrationCardExpiresAt")},
			"residenceDocType":       &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ResidenceDocType")},
			"residenceDocNumber":     &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ResidenceDocNumber")},
			"residenceDocIssuedAt":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ResidenceDocIssuedAt")},
			"residenceDocExpiresAt":  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ResidenceDocExpiresAt")},
		},
	})

	pdlDeclarationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PDLDeclaration",
		Fields: graphql.Fields{
			"isPdl":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("IsPDL")},
			"category": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Category")},
			"position": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Position")},
			"relation": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Relation")},
		},
	})

	representativeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Representative",
		Fields: graphql.Fields{
			"id":                  &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":            &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"legalEntityId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("LegalEntityID")},
			"lastName":            &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("LastName")},
			"firstName":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("FirstName")},
			"middleName":          &graphql.Field{Type: graphql.String, Resolve: fieldResolver("MiddleName")},
			"birthDate":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("BirthDate")},
			"birthPlace":          &graphql.Field{Type: graphql.String, Resolve: fieldResolver("BirthPlace")},
			"citizenship":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("Citizenship")},
			"inn":                 &graphql.Field{Type: graphql.String, Resolve: fieldResolver("INN")},
			"snils":               &graphql.Field{Type: graphql.String, Resolve: fieldResolver("SNILS")},
			"idDocument":          &graphql.Field{Type: graphql.NewNonNull(idDocumentType), Resolve: fieldResolver("IDDocument")},
			"registrationAddress": &graphql.Field{Type: structuredAddressType, Resolve: fieldResolver("RegistrationAddress")},
			"actualAddress":       &graphql.Field{Type: structuredAddressType, Resolve: fieldResolver("ActualAddress")},
			"foreignerInfo":       &graphql.Field{Type: foreignerInfoType, Resolve: fieldResolver("ForeignerInfo")},
			"authority":           &graphql.Field{Type: graphql.NewNonNull(authorityInfoType), Resolve: fieldResolver("Authority")},
			"pdlDeclaration":      &graphql.Field{Type: pdlDeclarationType, Resolve: fieldResolver("PDLDeclaration")},
			"isPrimary":           &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("IsPrimary")},
			"isSignatory":         &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("IsSignatory")},
			"createdAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	fatcaTinType := graphql.NewObject(graphql.ObjectConfig{
		Name: "FATCATIN",
		Fields: graphql.Fields{
			"country": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Country")},
			"tin":     &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("TIN")},
		},
	})

	fatcaDeclarationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "FATCADeclaration",
		Fields: graphql.Fields{
			"taxResidencyCountries": &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("TaxResidencyCountries")},
			"tinPerCountry":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(fatcaTinType)), Resolve: fieldResolver("TINPerCountry")},
			"usPerson":              &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("USPerson")},
			"formDocId":             &graphql.Field{Type: graphql.String, Resolve: fieldResolver("FormDocID")},
		},
	})

	uboNodeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBONode",
		Fields: graphql.Fields{
			"id":               &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"nodeType":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("NodeType")},
			"name":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Name")},
			"personId":         &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("PersonID")},
			"legalEntityId":    &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("LegalEntityID")},
			"directStake":      &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("DirectStake")},
			"effectiveStake":   &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("EffectiveStake")},
			"isUbo":            &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("IsUBO")},
			"controlBasis":     &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ControlBasis")},
			"fatcaDeclaration": &graphql.Field{Type: fatcaDeclarationType, Resolve: fieldResolver("FATCADeclaration")},
		},
	})

	uboEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOEdge",
		Fields: graphql.Fields{
			"fromNodeId": &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("FromNodeID")},
			"toNodeId":   &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ToNodeID")},
			"stake":      &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("Stake")},
			"documentId": &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("DocumentID")},
		},
	})

	ownershipChainLinkType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OwnershipChainLink",
		Fields: graphql.Fields{
			"level":                &graphql.Field{Type: graphql.NewNonNull(graphql.Int), Resolve: fieldResolver("Level")},
			"entityName":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("EntityName")},
			"entityInnOrRegNumber": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("EntityINNOrRegNumber")},
			"country":              &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Country")},
			"sharePercent":         &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: fieldResolver("SharePercent")},
		},
	})

	uboOwnershipChainType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOOwnershipChain",
		Fields: graphql.Fields{
			"uboNodeId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("UBONodeID")},
			"links":             &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(ownershipChainLinkType))), Resolve: fieldResolver("Links")},
			"isSoleBeneficiary": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("IsSoleBeneficiary")},
		},
	})

	uboGraphType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOGraph",
		Fields: graphql.Fields{
			"id":                   &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"legalEntityId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("LegalEntityID")},
			"nodes":                &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(uboNodeType))), Resolve: fieldResolver("Nodes")},
			"edges":                &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(uboEdgeType))), Resolve: fieldResolver("Edges")},
			"ownershipChains":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(uboOwnershipChainType)), Resolve: fieldResolver("OwnershipChains")},
			"noUboReason":          &graphql.Field{Type: graphql.String, Resolve: fieldResolver("NoUBOReason")},
			"eioAsUboConfirmation": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("EIOAsUBOConfirmation")},
			"diagramDocId":         &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("DiagramDocID")},
			"computedAt":           &graphql.Field{Type: timeScalar, Resolve: fieldResolver("ComputedAt")},
			"createdAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	screeningResultType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ScreeningResult",
		Fields: graphql.Fields{
			"listName":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("ListName")},
			"matchLevel":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("MatchLevel")},
			"score":             &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("Score")},
			"matchedEntityName": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("MatchedEntityName")},
			"checkedAt":         &graphql.Field{Type: timeScalar, Resolve: fieldResolver("CheckedAt")},
		},
	})

	adverseMediaHitType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AdverseMediaHit",
		Fields: graphql.Fields{
			"category":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Category")},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Title")},
			"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("URL")},
			"source":      &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Source")},
			"publishedAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("PublishedAt")},
			"summary":     &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Summary")},
		},
	})

	geoLocationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "GeoLocation",
		Fields: graphql.Fields{
			"country":   &graphql.Field{Type: graphql.String, Resolve: fieldResolver("Country")},
			"city":      &graphql.Field{Type: graphql.String, Resolve: fieldResolver("City")},
			"latitude":  &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("Latitude")},
			"longitude": &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("Longitude")},
		},
	})

	antiFraudSignalsType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AntiFraudSignals",
		Fields: graphql.Fields{
			"deviceFingerprint": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("DeviceFingerprint")},
			"ipAddress":         &graphql.Field{Type: graphql.String, Resolve: fieldResolver("IPAddress")},
			"geolocation":       &graphql.Field{Type: geoLocationType, Resolve: fieldResolver("Geolocation")},
		},
	})

	screeningResultSetType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ScreeningResultSet",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"sanctionsResults":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(screeningResultType)), Resolve: fieldResolver("SanctionsResults")},
			"pepResults":            &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(screeningResultType)), Resolve: fieldResolver("PEPResults")},
			"adverseMediaHits":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(adverseMediaHitType)), Resolve: fieldResolver("AdverseMediaHits")},
			"okvedConsistencyScore": &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("OKVEDConsistencyScore")},
			"turnoverRealismScore":  &graphql.Field{Type: graphql.Float, Resolve: fieldResolver("TurnoverRealismScore")},
			"antiFraudSignals":      &graphql.Field{Type: antiFraudSignalsType, Resolve: fieldResolver("AntiFraudSignals")},
			"performedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("PerformedAt")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	monitoringRuleType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MonitoringRule",
		Fields: graphql.Fields{
			"code":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Code")},
			"description": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Description")},
			"params":      &graphql.Field{Type: jsonScalar, Resolve: fieldResolver("Params")},
		},
	})

	transactionLimitsType := graphql.NewObject(graphql.ObjectConfig{
		Name: "TransactionLimits",
		Fields: graphql.Fields{
			"dailyOutgoing":        &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("DailyOutgoing")},
			"dailyCashWithdrawal":  &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("DailyCashWithdrawal")},
			"monthlyOutgoing":      &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("MonthlyOutgoing")},
			"singleTransactionMax": &graphql.Field{Type: moneyAmountType, Resolve: fieldResolver("SingleTransactionMax")},
		},
	})

	monitoringProfileType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MonitoringProfile",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"accountId":             &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("AccountID")},
			"reviewFrequencyMonths": &graphql.Field{Type: graphql.NewNonNull(graphql.Int), Resolve: fieldResolver("ReviewFrequencyMonths")},
			"nextReviewDate":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("NextReviewDate")},
			"monitoringRules":       &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(monitoringRuleType)), Resolve: fieldResolver("MonitoringRules")},
			"kycRefreshTriggers":    &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("KYCRefreshTriggers")},
			"transactionLimits":     &graphql.Field{Type: transactionLimitsType, Resolve: fieldResolver("TransactionLimits")},
			"notificationChannels":  &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("NotificationChannels")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
		},
	})

	accountAgreementsType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AccountAgreements",
		Fields: graphql.Fields{
			"agreementAcceptance":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("AgreementAcceptance")},
			"agreementAcceptedAt":   &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("AgreementAcceptedAt")},
			"dboAgreement":          &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("DBOAgreement")},
			"dboChannels":           &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: fieldResolver("DBOChannels")},
			"edoAgreement":          &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("EDOAgreement")},
			"personalDataConsent":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: fieldResolver("PersonalDataConsent")},
			"signingMethod":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("SigningMethod")},
			"ukepCertificateSerial": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("UKEPCertificateSerial")},
		},
	})

	bankAccountType := graphql.NewObject(graphql.ObjectConfig{
		Name: "BankAccount",
		Fields: graphql.Fields{
			"id":                   &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ID")},
			"tenantId":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("TenantID")},
			"applicationId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("ApplicationID")},
			"legalEntityId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: fieldResolver("LegalEntityID")},
			"accountNumber":        &graphql.Field{Type: graphql.String, Resolve: fieldResolver("AccountNumber")},
			"bik":                  &graphql.Field{Type: graphql.String, Resolve: fieldResolver("BIK")},
			"bankName":             &graphql.Field{Type: graphql.String, Resolve: fieldResolver("BankName")},
			"currency":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("Currency")},
			"accountType":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: fieldResolver("AccountType")},
			"correspondentAccount": &graphql.Field{Type: graphql.String, Resolve: fieldResolver("CorrespondentAccount")},
			"tariffPlan":           &graphql.Field{Type: graphql.String, Resolve: fieldResolver("TariffPlan")},
			"agreements":           &graphql.Field{Type: graphql.NewNonNull(accountAgreementsType), Resolve: fieldResolver("Agreements")},
			"monitoringProfileId":  &graphql.Field{Type: graphql.ID, Resolve: fieldResolver("MonitoringProfileID")},
			"absReference":         &graphql.Field{Type: graphql.String, Resolve: fieldResolver("ABSReference")},
			"openedAt":             &graphql.Field{Type: timeScalar, Resolve: fieldResolver("OpenedAt")},
			"createdAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("CreatedAt")},
			"updatedAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: fieldResolver("UpdatedAt")},
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
			"legalEntityProfile": &graphql.Field{
				Type:    legalEntityProfileType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveLegalEntityProfile,
			},
			"applicationActivity": &graphql.Field{
				Type:    applicationActivityType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveApplicationActivity,
			},
			"representatives": &graphql.Field{
				Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(representativeType))),
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveRepresentatives,
			},
			"uboGraph": &graphql.Field{
				Type:    uboGraphType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveUBOGraph,
			},
			"screening": &graphql.Field{
				Type:    screeningResultSetType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveScreening,
			},
			"monitoringProfile": &graphql.Field{
				Type:    monitoringProfileType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveMonitoringProfile,
			},
			"bankAccount": &graphql.Field{
				Type:    bankAccountType,
				Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
				Resolve: r.resolveBankAccount,
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

// ── Этапы 2-10 формы онбординга — резолверы ─────────────────────────
//
// Все резолверы следуют единому паттерну: достают tenant_id из AuthContext,
// валидируют applicationId, вызывают orchestrator-клиент. ErrNotFound
// (404 от orchestrator) → nil, nil — applicant ещё не дошёл до этапа,
// для GraphQL поле просто null, а не error.

func applicationIDArg(p graphql.ResolveParams) (string, error) {
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return "", fmt.Errorf("applicationId is required")
	}
	return id, nil
}

// resolveLegalEntityProfile — Query.legalEntityProfile (этап 2).
func (r *Resolver) resolveLegalEntityProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	prof, err := r.Orchestrator.GetLegalEntityProfile(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return prof, nil
}

// resolveApplicationActivity — Query.applicationActivity (этап 3).
func (r *Resolver) resolveApplicationActivity(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	act, err := r.Orchestrator.GetApplicationActivity(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return act, nil
}

// resolveRepresentatives — Query.representatives (этап 4).
// Список — пустой slice если applicant ещё не добавил представителей.
func (r *Resolver) resolveRepresentatives(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	reps, err := r.Orchestrator.ListRepresentatives(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return []clients.Representative{}, nil
		}
		return nil, err
	}
	if reps == nil {
		return []clients.Representative{}, nil
	}
	return reps, nil
}

// resolveUBOGraph — Query.uboGraph (этап 5).
func (r *Resolver) resolveUBOGraph(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	g, err := r.Orchestrator.GetUBOGraph(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

// resolveScreening — Query.screening (этап 7).
func (r *Resolver) resolveScreening(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	s, err := r.Orchestrator.GetScreening(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// resolveMonitoringProfile — Query.monitoringProfile (этапы 8/10).
func (r *Resolver) resolveMonitoringProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	mp, err := r.Orchestrator.GetMonitoringProfile(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return mp, nil
}

// resolveBankAccount — Query.bankAccount (этап 10).
func (r *Resolver) resolveBankAccount(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, err := applicationIDArg(p)
	if err != nil {
		return nil, err
	}
	a, err := r.Orchestrator.GetAccount(p.Context, ac.TenantID, id)
	if err != nil {
		if errors.Is(err, clients.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
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

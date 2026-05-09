// Package graph — резолверы для этапов 2-10 формы онбординга.
//
// Файл namespaced'нут от resolver.go, чтобы не превышать лимит 500 строк
// на файл (см. CLAUDE.md). Output/Input GraphQL-типы здесь —
// «полу-типизированные»: на верхнем уровне — конкретные поля и enum'ы,
// на глубоко-вложенных (адреса, FATCA, AccountAgreements,
// MonitoringRule.params) — scalar JSON, чтобы избежать дублирования
// 30+ struct из onboarding-orchestrator/internal/domain.
//
// Источник правды для shape'ов и валидаций — handler'ы в
// services/onboarding-orchestrator/internal/handler/{profile,activity,…}.go
// и $defs в packages/domain-model/schema.json (v1.1.0).
package graph

import (
	"fmt"

	"github.com/graphql-go/graphql"

	"aibank/bff-onboarding/internal/clients"
)

// formTypes — output- и input-объекты этапов 2-10. Собираются один раз
// при построении схемы и переиспользуются в queryFormFields/mutationFormFields.
type formTypes struct {
	timeScalar graphql.Type
	jsonScalar *graphql.Scalar

	// Output
	moneyAmount         *graphql.Object
	structuredAddress   *graphql.Object
	contactInfo         *graphql.Object
	licenseInfo         *graphql.Object
	sroMembership       *graphql.Object
	legalEntityProfile  *graphql.Object
	counterparty        *graphql.Object
	operationalModel    *graphql.Object
	fundsSource         *graphql.Object
	applicationActivity *graphql.Object
	idDocument          *graphql.Object
	authorityInfo       *graphql.Object
	foreignerInfo       *graphql.Object
	pdlDeclaration      *graphql.Object
	representative      *graphql.Object
	fatcaTIN            *graphql.Object
	fatcaDeclaration    *graphql.Object
	uboNode             *graphql.Object
	uboEdge             *graphql.Object
	chainLink           *graphql.Object
	ownershipChain      *graphql.Object
	uboGraph            *graphql.Object
	screeningResult     *graphql.Object
	adverseMediaHit     *graphql.Object
	geoLocation         *graphql.Object
	antiFraudSignals    *graphql.Object
	screeningResultSet  *graphql.Object
	monitoringRule      *graphql.Object
	transactionLimits   *graphql.Object
	monitoringProfile   *graphql.Object
	accountAgreements   *graphql.Object
	bankAccount         *graphql.Object

	// Input
	submitProfileInput    *graphql.InputObject
	submitActivityInput   *graphql.InputObject
	upsertRepInput        *graphql.InputObject
	upsertUboInput        *graphql.InputObject
	upsertScreeningInput  *graphql.InputObject
	upsertMonitoringInput *graphql.InputObject
	upsertAccountInput    *graphql.InputObject
}

// buildFormTypes конструирует все типы этапов 2-10. Вызывается из
// Resolver.Schema() ровно один раз. Ничего нельзя кэшировать в пакете —
// graphql-go не любит общие enum'ы между схемами.
func (r *Resolver) buildFormTypes(timeScalar graphql.Type, jsonScalar *graphql.Scalar) *formTypes {
	t := &formTypes{timeScalar: timeScalar, jsonScalar: jsonScalar}

	t.moneyAmount = graphql.NewObject(graphql.ObjectConfig{
		Name: "MoneyAmount",
		Fields: graphql.Fields{
			"amount":   &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"currency": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	t.structuredAddress = graphql.NewObject(graphql.ObjectConfig{
		Name: "StructuredAddress",
		Fields: graphql.Fields{
			"countryCode": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("country_code")},
			"postalCode":  &graphql.Field{Type: graphql.String, Resolve: jsonField("postal_code")},
			"regionCode":  &graphql.Field{Type: graphql.String, Resolve: jsonField("region_code")},
			"regionName":  &graphql.Field{Type: graphql.String, Resolve: jsonField("region_name")},
			"city":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("city")},
			"street":      &graphql.Field{Type: graphql.String, Resolve: jsonField("street")},
			"building":    &graphql.Field{Type: graphql.String, Resolve: jsonField("building")},
			"office":      &graphql.Field{Type: graphql.String, Resolve: jsonField("office")},
			"fiasId":      &graphql.Field{Type: graphql.String, Resolve: jsonField("fias_id")},
		},
	})

	t.licenseInfo = graphql.NewObject(graphql.ObjectConfig{
		Name: "LicenseInfo",
		Fields: graphql.Fields{
			"number":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("number")},
			"issueDate":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("issue_date")},
			"expiryDate":   &graphql.Field{Type: graphql.String, Resolve: jsonField("expiry_date")},
			"issuer":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("issuer")},
			"activityType": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("activity_type")},
		},
	})

	t.sroMembership = graphql.NewObject(graphql.ObjectConfig{
		Name: "SROMembership",
		Fields: graphql.Fields{
			"name":      &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("name")},
			"regNumber": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("reg_number")},
			"joinDate":  &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("join_date")},
		},
	})

	t.contactInfo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ContactInfo",
		Fields: graphql.Fields{
			"phone":   &graphql.Field{Type: graphql.String, Resolve: jsonField("phone")},
			"email":   &graphql.Field{Type: graphql.String, Resolve: jsonField("email")},
			"website": &graphql.Field{Type: graphql.String, Resolve: jsonField("website")},
		},
	})

	t.legalEntityProfile = graphql.NewObject(graphql.ObjectConfig{
		Name: "LegalEntityProfile",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"legalEntityId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("legal_entity_id")},
			"opfCode":               &graphql.Field{Type: graphql.String, Resolve: jsonField("opf_code")},
			"registrationAuthority": &graphql.Field{Type: graphql.String, Resolve: jsonField("registration_authority")},
			"authorizedCapital":     &graphql.Field{Type: t.moneyAmount, Resolve: jsonObjectField("authorized_capital", t.moneyAmount)},
			"legalAddress":          &graphql.Field{Type: t.structuredAddress, Resolve: jsonField("legal_address_struct")},
			"actualAddress":         &graphql.Field{Type: t.structuredAddress, Resolve: jsonField("actual_address_struct")},
			"actualSameAsLegal":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("actual_same_as_legal")},
			"postalAddress":         &graphql.Field{Type: t.structuredAddress, Resolve: jsonField("postal_address_struct")},
			"postalSameAsLegal":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("postal_same_as_legal")},
			"okvedMain":             &graphql.Field{Type: graphql.String, Resolve: jsonField("okved_main_v2")},
			"okvedAdditional":       &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("okved_additional_v2")},
			"licenses":              &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.licenseInfo)), Resolve: jsonField("licenses")},
			"sroMembership":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.sroMembership)), Resolve: jsonField("sro_membership")},
			"contacts":              &graphql.Field{Type: t.contactInfo, Resolve: jsonField("contacts")},
			"employeesCount":        &graphql.Field{Type: graphql.Int, Resolve: jsonField("employees_count")},
			"revenueLastYear":       &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("revenue_last_year")},
			"taxRegime":             &graphql.Field{Type: graphql.String, Resolve: jsonField("tax_regime")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.counterparty = graphql.NewObject(graphql.ObjectConfig{
		Name: "Counterparty",
		Fields: graphql.Fields{
			"name":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("name")},
			"inn":              &graphql.Field{Type: graphql.String, Resolve: jsonField("inn")},
			"country":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("country")},
			"sharePercent":     &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: jsonField("share_percent")},
			"relationshipType": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("relationship_type")},
		},
	})

	t.operationalModel = graphql.NewObject(graphql.ObjectConfig{
		Name: "OperationalModel",
		Fields: graphql.Fields{
			"geography":               &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("geography")},
			"monthlyTurnoverPlanned":  &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("monthly_turnover_planned")},
			"annualTurnoverPlanned":   &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("annual_turnover_planned")},
			"cashSharePercent":        &graphql.Field{Type: graphql.Float, Resolve: jsonField("cash_share_percent")},
			"foreignEconomicActivity": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("foreign_economic_activity")},
			"foreignCountries":        &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("foreign_countries")},
			"currencyOperations":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("currency_operations")},
		},
	})

	t.fundsSource = graphql.NewObject(graphql.ObjectConfig{
		Name: "FundsSource",
		Fields: graphql.Fields{
			"category":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("category")},
			"description": &graphql.Field{Type: graphql.String, Resolve: jsonField("description")},
		},
	})

	t.applicationActivity = graphql.NewObject(graphql.ObjectConfig{
		Name: "ApplicationActivity",
		Fields: graphql.Fields{
			"id":                  &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":            &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"businessDescription": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("business_description")},
			"businessCategory":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("business_category")},
			"topSuppliers":        &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.counterparty)), Resolve: jsonField("top_suppliers")},
			"topBuyers":           &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.counterparty)), Resolve: jsonField("top_buyers")},
			"operationalModel":    &graphql.Field{Type: t.operationalModel, Resolve: jsonField("operational_model")},
			"fundsSource":         &graphql.Field{Type: graphql.NewNonNull(t.fundsSource), Resolve: jsonField("funds_source")},
			"createdAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.idDocument = graphql.NewObject(graphql.ObjectConfig{
		Name: "IDDocument",
		Fields: graphql.Fields{
			"docType":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("doc_type")},
			"series":         &graphql.Field{Type: graphql.String, Resolve: jsonField("series")},
			"number":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("number")},
			"issueDate":      &graphql.Field{Type: graphql.String, Resolve: jsonField("issue_date")},
			"expiryDate":     &graphql.Field{Type: graphql.String, Resolve: jsonField("expiry_date")},
			"issuedBy":       &graphql.Field{Type: graphql.String, Resolve: jsonField("issued_by")},
			"departmentCode": &graphql.Field{Type: graphql.String, Resolve: jsonField("department_code")},
		},
	})

	t.authorityInfo = graphql.NewObject(graphql.ObjectConfig{
		Name: "AuthorityInfo",
		Fields: graphql.Fields{
			"position":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("position")},
			"authorityBasis":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("authority_basis")},
			"authorityDocNumber":   &graphql.Field{Type: graphql.String, Resolve: jsonField("authority_doc_number")},
			"authorityDocDate":     &graphql.Field{Type: graphql.String, Resolve: jsonField("authority_doc_date")},
			"signatureSampleDocId": &graphql.Field{Type: graphql.String, Resolve: jsonField("signature_sample_doc_id")},
		},
	})

	t.foreignerInfo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ForeignerInfo",
		Fields: graphql.Fields{
			"migrationCardNumber":    &graphql.Field{Type: graphql.String, Resolve: jsonField("migration_card_number")},
			"migrationCardIssuedAt":  &graphql.Field{Type: graphql.String, Resolve: jsonField("migration_card_issued_at")},
			"migrationCardExpiresAt": &graphql.Field{Type: graphql.String, Resolve: jsonField("migration_card_expires_at")},
			"residenceDocType":       &graphql.Field{Type: graphql.String, Resolve: jsonField("residence_doc_type")},
			"residenceDocNumber":     &graphql.Field{Type: graphql.String, Resolve: jsonField("residence_doc_number")},
			"residenceDocIssuedAt":   &graphql.Field{Type: graphql.String, Resolve: jsonField("residence_doc_issued_at")},
			"residenceDocExpiresAt":  &graphql.Field{Type: graphql.String, Resolve: jsonField("residence_doc_expires_at")},
		},
	})

	t.pdlDeclaration = graphql.NewObject(graphql.ObjectConfig{
		Name: "PDLDeclaration",
		Fields: graphql.Fields{
			"isPdl":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("is_pdl")},
			"category": &graphql.Field{Type: graphql.String, Resolve: jsonField("category")},
			"position": &graphql.Field{Type: graphql.String, Resolve: jsonField("position")},
			"relation": &graphql.Field{Type: graphql.String, Resolve: jsonField("relation")},
		},
	})

	t.representative = graphql.NewObject(graphql.ObjectConfig{
		Name: "Representative",
		Fields: graphql.Fields{
			"id":                  &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":            &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"legalEntityId":       &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("legal_entity_id")},
			"lastName":            &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("last_name")},
			"firstName":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("first_name")},
			"middleName":          &graphql.Field{Type: graphql.String, Resolve: jsonField("middle_name")},
			"birthDate":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("birth_date")},
			"birthPlace":          &graphql.Field{Type: graphql.String, Resolve: jsonField("birth_place")},
			"citizenship":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("citizenship")},
			"inn":                 &graphql.Field{Type: graphql.String, Resolve: jsonField("inn")},
			"snils":               &graphql.Field{Type: graphql.String, Resolve: jsonField("snils")},
			"idDocument":          &graphql.Field{Type: graphql.NewNonNull(t.idDocument), Resolve: jsonField("id_document")},
			"registrationAddress": &graphql.Field{Type: t.structuredAddress, Resolve: jsonField("registration_address")},
			"actualAddress":       &graphql.Field{Type: t.structuredAddress, Resolve: jsonField("actual_address")},
			"foreignerInfo":       &graphql.Field{Type: t.foreignerInfo, Resolve: jsonField("foreigner_info")},
			"authority":           &graphql.Field{Type: graphql.NewNonNull(t.authorityInfo), Resolve: jsonField("authority")},
			"pdlDeclaration":      &graphql.Field{Type: t.pdlDeclaration, Resolve: jsonField("pdl_declaration")},
			"isPrimary":           &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("is_primary")},
			"isSignatory":         &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("is_signatory")},
			"createdAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.fatcaTIN = graphql.NewObject(graphql.ObjectConfig{
		Name: "FATCATIN",
		Fields: graphql.Fields{
			"country": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("country")},
			"tin":     &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tin")},
		},
	})
	t.fatcaDeclaration = graphql.NewObject(graphql.ObjectConfig{
		Name: "FATCADeclaration",
		Fields: graphql.Fields{
			"taxResidencyCountries": &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("tax_residency_countries")},
			"tinPerCountry":         &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.fatcaTIN)), Resolve: jsonField("tin_per_country")},
			"usPerson":              &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("us_person")},
			"formDocId":             &graphql.Field{Type: graphql.String, Resolve: jsonField("form_doc_id")},
		},
	})

	t.uboNode = graphql.NewObject(graphql.ObjectConfig{
		Name: "UBONode",
		Fields: graphql.Fields{
			"id":               &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"nodeType":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("node_type")},
			"name":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("name")},
			"personId":         &graphql.Field{Type: graphql.String, Resolve: jsonField("person_id")},
			"legalEntityId":    &graphql.Field{Type: graphql.String, Resolve: jsonField("legal_entity_id")},
			"directStake":      &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: jsonField("direct_stake")},
			"effectiveStake":   &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: jsonField("effective_stake")},
			"isUbo":            &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("is_ubo")},
			"controlBasis":     &graphql.Field{Type: graphql.String, Resolve: jsonField("control_basis")},
			"fatcaDeclaration": &graphql.Field{Type: t.fatcaDeclaration, Resolve: jsonField("fatca_declaration")},
		},
	})
	t.uboEdge = graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOEdge",
		Fields: graphql.Fields{
			"fromNodeId": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("from_node_id")},
			"toNodeId":   &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("to_node_id")},
			"stake":      &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: jsonField("stake")},
			"documentId": &graphql.Field{Type: graphql.String, Resolve: jsonField("document_id")},
		},
	})
	t.chainLink = graphql.NewObject(graphql.ObjectConfig{
		Name: "OwnershipChainLink",
		Fields: graphql.Fields{
			"level":                &graphql.Field{Type: graphql.NewNonNull(graphql.Int), Resolve: jsonField("level")},
			"entityName":           &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("entity_name")},
			"entityInnOrRegNumber": &graphql.Field{Type: graphql.String, Resolve: jsonField("entity_inn_or_reg_number")},
			"country":              &graphql.Field{Type: graphql.String, Resolve: jsonField("country")},
			"sharePercent":         &graphql.Field{Type: graphql.NewNonNull(graphql.Float), Resolve: jsonField("share_percent")},
		},
	})
	t.ownershipChain = graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOOwnershipChain",
		Fields: graphql.Fields{
			"uboNodeId":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("ubo_node_id")},
			"links":             &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(t.chainLink))), Resolve: jsonField("links")},
			"isSoleBeneficiary": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("is_sole_beneficiary")},
		},
	})

	t.uboGraph = graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOGraph",
		Fields: graphql.Fields{
			"id":                   &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"legalEntityId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("legal_entity_id")},
			"nodes":                &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(t.uboNode))), Resolve: jsonField("nodes")},
			"edges":                &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(t.uboEdge))), Resolve: jsonField("edges")},
			"ownershipChains":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.ownershipChain)), Resolve: jsonField("ownership_chains")},
			"noUboReason":          &graphql.Field{Type: graphql.String, Resolve: jsonField("no_ubo_reason")},
			"eioAsUboConfirmation": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("eio_as_ubo_confirmation")},
			"diagramDocId":         &graphql.Field{Type: graphql.String, Resolve: jsonField("diagram_doc_id")},
			"computedAt":           &graphql.Field{Type: timeScalar, Resolve: jsonTimeField("computed_at")},
			"createdAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":            &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.screeningResult = graphql.NewObject(graphql.ObjectConfig{
		Name: "ScreeningResult",
		Fields: graphql.Fields{
			"listName":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("list_name")},
			"matchLevel":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("match_level")},
			"score":             &graphql.Field{Type: graphql.Float, Resolve: jsonField("score")},
			"matchedEntityName": &graphql.Field{Type: graphql.String, Resolve: jsonField("matched_entity_name")},
			"checkedAt":         &graphql.Field{Type: timeScalar, Resolve: jsonTimeField("checked_at")},
		},
	})
	t.adverseMediaHit = graphql.NewObject(graphql.ObjectConfig{
		Name: "AdverseMediaHit",
		Fields: graphql.Fields{
			"category":    &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("category")},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("title")},
			"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("url")},
			"source":      &graphql.Field{Type: graphql.String, Resolve: jsonField("source")},
			"publishedAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("published_at")},
			"summary":     &graphql.Field{Type: graphql.String, Resolve: jsonField("summary")},
		},
	})
	t.geoLocation = graphql.NewObject(graphql.ObjectConfig{
		Name: "GeoLocation",
		Fields: graphql.Fields{
			"country":   &graphql.Field{Type: graphql.String, Resolve: jsonField("country")},
			"city":      &graphql.Field{Type: graphql.String, Resolve: jsonField("city")},
			"latitude":  &graphql.Field{Type: graphql.Float, Resolve: jsonField("latitude")},
			"longitude": &graphql.Field{Type: graphql.Float, Resolve: jsonField("longitude")},
		},
	})
	t.antiFraudSignals = graphql.NewObject(graphql.ObjectConfig{
		Name: "AntiFraudSignals",
		Fields: graphql.Fields{
			"deviceFingerprint": &graphql.Field{Type: graphql.String, Resolve: jsonField("device_fingerprint")},
			"ipAddress":         &graphql.Field{Type: graphql.String, Resolve: jsonField("ip_address")},
			"geolocation":       &graphql.Field{Type: t.geoLocation, Resolve: jsonField("geolocation")},
		},
	})
	t.screeningResultSet = graphql.NewObject(graphql.ObjectConfig{
		Name: "ScreeningResultSet",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"sanctionsResults":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.screeningResult)), Resolve: jsonField("sanctions_results")},
			"pepResults":            &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.screeningResult)), Resolve: jsonField("pep_results")},
			"adverseMediaHits":      &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.adverseMediaHit)), Resolve: jsonField("adverse_media_hits")},
			"okvedConsistencyScore": &graphql.Field{Type: graphql.Float, Resolve: jsonField("okved_consistency_score")},
			"turnoverRealismScore":  &graphql.Field{Type: graphql.Float, Resolve: jsonField("turnover_realism_score")},
			"antiFraudSignals":      &graphql.Field{Type: t.antiFraudSignals, Resolve: jsonField("anti_fraud_signals")},
			"performedAt":           &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("performed_at")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.monitoringRule = graphql.NewObject(graphql.ObjectConfig{
		Name: "MonitoringRule",
		Fields: graphql.Fields{
			"code":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("code")},
			"description": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("description")},
			"params":      &graphql.Field{Type: jsonScalar, Resolve: jsonField("params")},
		},
	})
	t.transactionLimits = graphql.NewObject(graphql.ObjectConfig{
		Name: "TransactionLimits",
		Fields: graphql.Fields{
			"dailyOutgoing":        &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("daily_outgoing")},
			"dailyCashWithdrawal":  &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("daily_cash_withdrawal")},
			"monthlyOutgoing":      &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("monthly_outgoing")},
			"singleTransactionMax": &graphql.Field{Type: t.moneyAmount, Resolve: jsonField("single_transaction_max")},
		},
	})
	t.monitoringProfile = graphql.NewObject(graphql.ObjectConfig{
		Name: "MonitoringProfile",
		Fields: graphql.Fields{
			"id":                    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":              &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"accountId":             &graphql.Field{Type: graphql.String, Resolve: jsonField("account_id")},
			"reviewFrequencyMonths": &graphql.Field{Type: graphql.NewNonNull(graphql.Int), Resolve: jsonField("review_frequency_months")},
			"nextReviewDate":        &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("next_review_date")},
			"monitoringRules":       &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(t.monitoringRule)), Resolve: jsonField("monitoring_rules")},
			"kycRefreshTriggers":    &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("kyc_refresh_triggers")},
			"transactionLimits":     &graphql.Field{Type: t.transactionLimits, Resolve: jsonField("transaction_limits")},
			"notificationChannels":  &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("notification_channels")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	t.accountAgreements = graphql.NewObject(graphql.ObjectConfig{
		Name: "AccountAgreements",
		Fields: graphql.Fields{
			"agreementAcceptance":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("agreement_acceptance")},
			"agreementAcceptedAt":   &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("agreement_accepted_at")},
			"dboAgreement":          &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("dbo_agreement")},
			"dboChannels":           &graphql.Field{Type: graphql.NewList(graphql.NewNonNull(graphql.String)), Resolve: jsonField("dbo_channels")},
			"edoAgreement":          &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("edo_agreement")},
			"personalDataConsent":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean), Resolve: jsonField("personal_data_consent")},
			"signingMethod":         &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("signing_method")},
			"ukepCertificateSerial": &graphql.Field{Type: graphql.String, Resolve: jsonField("ukep_certificate_serial")},
		},
	})
	t.bankAccount = graphql.NewObject(graphql.ObjectConfig{
		Name: "BankAccount",
		Fields: graphql.Fields{
			"id":                   &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("id")},
			"tenantId":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("tenant_id")},
			"applicationId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("application_id")},
			"legalEntityId":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: jsonField("legal_entity_id")},
			"accountNumber":        &graphql.Field{Type: graphql.String, Resolve: jsonField("account_number")},
			"bik":                  &graphql.Field{Type: graphql.String, Resolve: jsonField("bik")},
			"bankName":             &graphql.Field{Type: graphql.String, Resolve: jsonField("bank_name")},
			"currency":             &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("currency")},
			"accountType":          &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: jsonField("account_type")},
			"correspondentAccount": &graphql.Field{Type: graphql.String, Resolve: jsonField("correspondent_account")},
			"tariffPlan":           &graphql.Field{Type: graphql.String, Resolve: jsonField("tariff_plan")},
			"agreements":           &graphql.Field{Type: graphql.NewNonNull(t.accountAgreements), Resolve: jsonField("agreements")},
			"monitoringProfileId":  &graphql.Field{Type: graphql.String, Resolve: jsonField("monitoring_profile_id")},
			"absReference":         &graphql.Field{Type: graphql.String, Resolve: jsonField("abs_reference")},
			"openedAt":             &graphql.Field{Type: timeScalar, Resolve: jsonTimeField("opened_at")},
			"createdAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("created_at")},
			"updatedAt":             &graphql.Field{Type: graphql.NewNonNull(timeScalar), Resolve: jsonTimeField("updated_at")},
		},
	})

	// ── Input types ────────────────────────────────────────────────
	// Глубоко-вложенные поля принимаем как scalar JSON: handler
	// orchestrator'а валидирует структуру, а схема BFF остаётся компактной.

	t.submitProfileInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SubmitLegalEntityProfileInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"legalEntityId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"opfCode":               &graphql.InputObjectFieldConfig{Type: graphql.String},
			"registrationAuthority": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"authorizedCapital":     &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"legalAddressStruct":    &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"actualAddressStruct":   &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"actualSameAsLegal":     &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"postalAddressStruct":   &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"postalSameAsLegal":     &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"okvedMainV2":           &graphql.InputObjectFieldConfig{Type: graphql.String},
			"okvedAdditionalV2":     &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(graphql.String))},
			"licenses":              &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"sroMembership":         &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"contacts":              &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"employeesCount":        &graphql.InputObjectFieldConfig{Type: graphql.Int},
			"revenueLastYear":       &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"taxRegime":             &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	t.submitActivityInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SubmitApplicationActivityInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"businessDescription": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"businessCategory":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"topSuppliers":        &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"topBuyers":           &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"operationalModel":    &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"fundsSource":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(jsonScalar)},
		},
	})
	t.upsertRepInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpsertRepresentativeInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"id":                  &graphql.InputObjectFieldConfig{Type: graphql.ID},
			"applicationId":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"legalEntityId":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"lastName":            &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"firstName":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"middleName":          &graphql.InputObjectFieldConfig{Type: graphql.String},
			"birthDate":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"birthPlace":          &graphql.InputObjectFieldConfig{Type: graphql.String},
			"citizenship":         &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(graphql.String))},
			"inn":                 &graphql.InputObjectFieldConfig{Type: graphql.String},
			"snils":               &graphql.InputObjectFieldConfig{Type: graphql.String},
			"idDocument":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(jsonScalar)},
			"registrationAddress": &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"actualAddress":       &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"foreignerInfo":       &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"authority":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(jsonScalar)},
			"pdlDeclaration":      &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"isPrimary":           &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"isSignatory":         &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
		},
	})
	t.upsertUboInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpsertUBOGraphInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"legalEntityId":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"nodes":                &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(jsonScalar)))},
			"edges":                &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(jsonScalar)))},
			"ownershipChains":      &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"noUboReason":          &graphql.InputObjectFieldConfig{Type: graphql.String},
			"eioAsUboConfirmation": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"diagramDocId":         &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	t.upsertScreeningInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpsertScreeningInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"sanctionsResults":      &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"pepResults":            &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"adverseMediaHits":      &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"okvedConsistencyScore": &graphql.InputObjectFieldConfig{Type: graphql.Float},
			"turnoverRealismScore":  &graphql.InputObjectFieldConfig{Type: graphql.Float},
			"antiFraudSignals":      &graphql.InputObjectFieldConfig{Type: jsonScalar},
		},
	})
	t.upsertMonitoringInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpsertMonitoringProfileInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"accountId":             &graphql.InputObjectFieldConfig{Type: graphql.String},
			"reviewFrequencyMonths": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
			"nextReviewDate":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"monitoringRules":       &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(jsonScalar))},
			"kycRefreshTriggers":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(graphql.String))},
			"transactionLimits":     &graphql.InputObjectFieldConfig{Type: jsonScalar},
			"notificationChannels":  &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(graphql.String))},
		},
	})
	t.upsertAccountInput = graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpsertBankAccountInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"applicationId":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"legalEntityId":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"accountNumber":        &graphql.InputObjectFieldConfig{Type: graphql.String},
			"bik":                  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"bankName":             &graphql.InputObjectFieldConfig{Type: graphql.String},
			"currency":             &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"accountType":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"correspondentAccount": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"tariffPlan":           &graphql.InputObjectFieldConfig{Type: graphql.String},
			"agreements":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(jsonScalar)},
			"monitoringProfileId":  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"absReference":         &graphql.InputObjectFieldConfig{Type: graphql.String},
			"openedAt":             &graphql.InputObjectFieldConfig{Type: timeScalar},
		},
	})

	return t
}

// queryFormFields — поля Query этапов 2-10. Подмешиваются в graphql.Fields
// внутри Resolver.Schema().
func (r *Resolver) queryFormFields(t *formTypes) graphql.Fields {
	return graphql.Fields{
		"legalEntityProfile": &graphql.Field{
			Type:    t.legalEntityProfile,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveLegalEntityProfile,
		},
		"applicationActivity": &graphql.Field{
			Type:    t.applicationActivity,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveApplicationActivity,
		},
		"representatives": &graphql.Field{
			Type:    graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(t.representative))),
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveListRepresentatives,
		},
		"representative": &graphql.Field{
			Type:    t.representative,
			Args:    graphql.FieldConfigArgument{"id": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveGetRepresentative,
		},
		"uboGraph": &graphql.Field{
			Type:    t.uboGraph,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveUBOGraph,
		},
		"screening": &graphql.Field{
			Type:    t.screeningResultSet,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveScreening,
		},
		"monitoringProfile": &graphql.Field{
			Type:    t.monitoringProfile,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveMonitoringProfile,
		},
		"bankAccount": &graphql.Field{
			Type:    t.bankAccount,
			Args:    graphql.FieldConfigArgument{"applicationId": {Type: graphql.NewNonNull(graphql.ID)}},
			Resolve: r.resolveBankAccount,
		},
	}
}

// mutationFormFields — поля Mutation этапов 2-10.
func (r *Resolver) mutationFormFields(t *formTypes) graphql.Fields {
	return graphql.Fields{
		"submitLegalEntityProfile": &graphql.Field{
			Type:    graphql.NewNonNull(t.legalEntityProfile),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.submitProfileInput)}},
			Resolve: r.resolveSubmitLegalEntityProfile,
		},
		"submitApplicationActivity": &graphql.Field{
			Type:    graphql.NewNonNull(t.applicationActivity),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.submitActivityInput)}},
			Resolve: r.resolveSubmitApplicationActivity,
		},
		"upsertRepresentative": &graphql.Field{
			Type:    graphql.NewNonNull(t.representative),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.upsertRepInput)}},
			Resolve: r.resolveUpsertRepresentative,
		},
		"upsertUboGraph": &graphql.Field{
			Type:    graphql.NewNonNull(t.uboGraph),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.upsertUboInput)}},
			Resolve: r.resolveUpsertUBOGraph,
		},
		"upsertScreening": &graphql.Field{
			Type:    graphql.NewNonNull(t.screeningResultSet),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.upsertScreeningInput)}},
			Resolve: r.resolveUpsertScreening,
		},
		"upsertMonitoringProfile": &graphql.Field{
			Type:    graphql.NewNonNull(t.monitoringProfile),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.upsertMonitoringInput)}},
			Resolve: r.resolveUpsertMonitoringProfile,
		},
		"upsertBankAccount": &graphql.Field{
			Type:    graphql.NewNonNull(t.bankAccount),
			Args:    graphql.FieldConfigArgument{"input": {Type: graphql.NewNonNull(t.upsertAccountInput)}},
			Resolve: r.resolveUpsertBankAccount,
		},
	}
}

// ── Resolver implementations ──────────────────────────────────────────

// inputMap извлекает из p.Args["input"] map[string]interface{} либо
// отдаёт ошибку. Тонкий хелпер, чтобы ниже было меньше шума.
func inputMap(p graphql.ResolveParams) (map[string]interface{}, error) {
	in, _ := p.Args["input"].(map[string]interface{})
	if in == nil {
		return nil, fmt.Errorf("input is required")
	}
	return in, nil
}

// nilOnNotFound маппит clients.ErrNotFound в (nil, nil) — для
// «mayBe-null» query-полей вроде legalEntityProfile, screening и т.д.
func nilOnNotFound[T any](v T, err error) (interface{}, error) {
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func isNotFound(err error) bool {
	for e := err; e != nil; {
		if e == clients.ErrNotFound {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// ── Этап 2 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveSubmitLegalEntityProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	body := profileBody(in)
	return r.Orchestrator.SubmitLegalEntityProfile(p.Context, ac.TenantID, body)
}

func (r *Resolver) resolveLegalEntityProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetLegalEntityProfile(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этап 3 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveSubmitApplicationActivity(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.SubmitApplicationActivity(p.Context, ac.TenantID, activityBody(in))
}

func (r *Resolver) resolveApplicationActivity(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetApplicationActivity(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этап 4 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveUpsertRepresentative(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.UpsertRepresentative(p.Context, ac.TenantID, representativeBody(in))
}

func (r *Resolver) resolveListRepresentatives(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	items, err := r.Orchestrator.ListRepresentatives(p.Context, ac.TenantID, id)
	if err != nil {
		if isNotFound(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	return items, nil
}

func (r *Resolver) resolveGetRepresentative(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	v, err := r.Orchestrator.GetRepresentative(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этап 5 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveUpsertUBOGraph(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.UpsertUBOGraph(p.Context, ac.TenantID, uboBody(in))
}

func (r *Resolver) resolveUBOGraph(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetUBOGraph(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этап 7 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveUpsertScreening(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.UpsertScreening(p.Context, ac.TenantID, screeningBody(in))
}

func (r *Resolver) resolveScreening(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetScreening(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этапы 8/10 ────────────────────────────────────────────────────────

func (r *Resolver) resolveUpsertMonitoringProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.UpsertMonitoringProfile(p.Context, ac.TenantID, monitoringBody(in))
}

func (r *Resolver) resolveMonitoringProfile(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetMonitoringProfile(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Этап 9 ────────────────────────────────────────────────────────────

func (r *Resolver) resolveUpsertBankAccount(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	in, err := inputMap(p)
	if err != nil {
		return nil, err
	}
	return r.Orchestrator.UpsertAccount(p.Context, ac.TenantID, accountBody(in))
}

func (r *Resolver) resolveBankAccount(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	id, _ := p.Args["applicationId"].(string)
	if id == "" {
		return nil, fmt.Errorf("applicationId is required")
	}
	v, err := r.Orchestrator.GetAccount(p.Context, ac.TenantID, id)
	return nilOnNotFound(v, err)
}

// ── Body-builders: mapping camelCase GraphQL inputs → snake_case JSON ──
//
// Поля, которые orchestrator ждёт под другим snake_case именем (camelCase
// в GraphQL → snake_case в orchestrator REST), переименовываются здесь.
// Глубоко-вложенные поля приходят как scalar JSON (interface{}) и
// прокидываются «как есть» — orchestrator валидирует их сам.

func profileBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":   in["applicationId"],
		"legal_entity_id":  in["legalEntityId"],
		"actual_same_as_legal": valueOrFalse(in["actualSameAsLegal"]),
		"postal_same_as_legal": valueOrFalse(in["postalSameAsLegal"]),
	}
	copyIfPresent(body, in, map[string]string{
		"opfCode":               "opf_code",
		"registrationAuthority": "registration_authority",
		"authorizedCapital":     "authorized_capital",
		"legalAddressStruct":    "legal_address_struct",
		"actualAddressStruct":   "actual_address_struct",
		"postalAddressStruct":   "postal_address_struct",
		"okvedMainV2":           "okved_main_v2",
		"okvedAdditionalV2":     "okved_additional_v2",
		"licenses":              "licenses",
		"sroMembership":         "sro_membership",
		"contacts":              "contacts",
		"employeesCount":        "employees_count",
		"revenueLastYear":       "revenue_last_year",
		"taxRegime":             "tax_regime",
	})
	return body
}

func activityBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":       in["applicationId"],
		"business_description": in["businessDescription"],
		"business_category":    in["businessCategory"],
		"funds_source":         in["fundsSource"],
	}
	copyIfPresent(body, in, map[string]string{
		"topSuppliers":     "top_suppliers",
		"topBuyers":        "top_buyers",
		"operationalModel": "operational_model",
	})
	return body
}

func representativeBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":   in["applicationId"],
		"legal_entity_id":  in["legalEntityId"],
		"last_name":        in["lastName"],
		"first_name":       in["firstName"],
		"birth_date":       in["birthDate"],
		"id_document":      in["idDocument"],
		"authority":        in["authority"],
		"is_primary":       valueOrFalse(in["isPrimary"]),
		"is_signatory":     valueOrFalse(in["isSignatory"]),
	}
	copyIfPresent(body, in, map[string]string{
		"id":                  "id",
		"middleName":          "middle_name",
		"birthPlace":          "birth_place",
		"citizenship":         "citizenship",
		"inn":                 "inn",
		"snils":               "snils",
		"registrationAddress": "registration_address",
		"actualAddress":       "actual_address",
		"foreignerInfo":       "foreigner_info",
		"pdlDeclaration":      "pdl_declaration",
	})
	return body
}

func uboBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":          in["applicationId"],
		"legal_entity_id":         in["legalEntityId"],
		"nodes":                   in["nodes"],
		"edges":                   in["edges"],
		"eio_as_ubo_confirmation": valueOrFalse(in["eioAsUboConfirmation"]),
	}
	copyIfPresent(body, in, map[string]string{
		"ownershipChains": "ownership_chains",
		"noUboReason":     "no_ubo_reason",
		"diagramDocId":    "diagram_doc_id",
	})
	return body
}

func screeningBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id": in["applicationId"],
	}
	copyIfPresent(body, in, map[string]string{
		"sanctionsResults":      "sanctions_results",
		"pepResults":            "pep_results",
		"adverseMediaHits":      "adverse_media_hits",
		"okvedConsistencyScore": "okved_consistency_score",
		"turnoverRealismScore":  "turnover_realism_score",
		"antiFraudSignals":      "anti_fraud_signals",
	})
	return body
}

func monitoringBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":          in["applicationId"],
		"review_frequency_months": in["reviewFrequencyMonths"],
		"next_review_date":        in["nextReviewDate"],
	}
	copyIfPresent(body, in, map[string]string{
		"accountId":            "account_id",
		"monitoringRules":      "monitoring_rules",
		"kycRefreshTriggers":   "kyc_refresh_triggers",
		"transactionLimits":    "transaction_limits",
		"notificationChannels": "notification_channels",
	})
	return body
}

func accountBody(in map[string]interface{}) map[string]any {
	body := map[string]any{
		"application_id":  in["applicationId"],
		"legal_entity_id": in["legalEntityId"],
		"currency":        in["currency"],
		"account_type":    in["accountType"],
		"agreements":      in["agreements"],
	}
	copyIfPresent(body, in, map[string]string{
		"accountNumber":        "account_number",
		"bik":                  "bik",
		"bankName":             "bank_name",
		"correspondentAccount": "correspondent_account",
		"tariffPlan":           "tariff_plan",
		"monitoringProfileId":  "monitoring_profile_id",
		"absReference":         "abs_reference",
		"openedAt":             "opened_at",
	})
	return body
}

func copyIfPresent(dst, src map[string]interface{}, mapping map[string]string) {
	for from, to := range mapping {
		if v, ok := src[from]; ok && v != nil {
			dst[to] = v
		}
	}
}

func valueOrFalse(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

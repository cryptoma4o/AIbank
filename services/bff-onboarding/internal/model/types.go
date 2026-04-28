// Package model — Go-типы доменной модели BFF.
//
// Эти типы — runtime-представление сущностей из graph/schema.graphqls.
// При переходе на gqlgen-codegen (см. gqlgen.yml) часть типов будет
// продублирована в internal/model/types_gen.go; ручные типы здесь
// останутся authoritative для enum'ов и DTO, не переопределяемых
// схемой.
package model

import "time"

// ApplicationState — зеркало onboarding-orchestrator/internal/domain.ApplicationState.
type ApplicationState string

const (
	StateDraft               ApplicationState = "draft"
	StateIdentifying         ApplicationState = "identifying"
	StateCollectingDocuments ApplicationState = "collecting_documents"
	StateValidating          ApplicationState = "validating"
	StateWaitingForClient    ApplicationState = "waiting_for_client"
	StateRiskAssessing       ApplicationState = "risk_assessing"
	StateAutoApproved        ApplicationState = "auto_approved"
	StateManualReview        ApplicationState = "manual_review"
	StateApproved            ApplicationState = "approved"
	StateApprovedWithEDD     ApplicationState = "approved_with_edd"
	StateRequiresMoreInfo    ApplicationState = "requires_more_info"
	StateOpeningAccount      ApplicationState = "opening_account"
	StateAccountOpened       ApplicationState = "account_opened"
	StateDeclined            ApplicationState = "declined"
	StateAbandoned           ApplicationState = "abandoned"
)

// DocumentType — тип документа.
type DocumentType string

const (
	DocPassport         DocumentType = "PASSPORT"
	DocCharter          DocumentType = "CHARTER"
	DocProtocol         DocumentType = "PROTOCOL"
	DocAgreement        DocumentType = "AGREEMENT"
	DocEgrulExtract     DocumentType = "EGRUL_EXTRACT"
	DocPowerOfAttorney  DocumentType = "POWER_OF_ATTORNEY"
	DocAccountingReport DocumentType = "ACCOUNTING_REPORT"
	DocOther            DocumentType = "OTHER"
)

// RiskCategory — итоговая категория риска.
type RiskCategory string

const (
	RiskLow    RiskCategory = "LOW"
	RiskMedium RiskCategory = "MEDIUM"
	RiskHigh   RiskCategory = "HIGH"
)

// DecisionKind — финальное решение.
type DecisionKind string

const (
	DecisionApproved        DecisionKind = "APPROVED"
	DecisionApprovedWithEDD DecisionKind = "APPROVED_WITH_EDD"
	DecisionDeclined        DecisionKind = "DECLINED"
	DecisionEscalated       DecisionKind = "ESCALATED"
)

// Me — представление текущего пользователя по JWT.
type Me struct {
	UserID   string `json:"userId"`
	TenantID string `json:"tenantId"`
	Role     string `json:"role"`
	Email    string `json:"email,omitempty"`
}

// Tenant — урезанный shape тенанта для UI клиента.
type Tenant struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BIK            string `json:"bik"`
	INN            string `json:"inn"`
	Status         string `json:"status"`
	DeploymentMode string `json:"deploymentMode"`
}

// Person — физлицо.
type Person struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	INN      string `json:"inn,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
}

// Document — документ, привязанный к заявке.
type Document struct {
	ID            string       `json:"id"`
	Type          DocumentType `json:"type"`
	ApplicationID string       `json:"applicationId"`
	Filename      string       `json:"filename"`
	State         string       `json:"state"`
	UploadedAt    time.Time    `json:"uploadedAt"`
}

// RiskAssessment — оценка риска.
type RiskAssessment struct {
	ID             string                   `json:"id"`
	ApplicationID  string                   `json:"applicationId"`
	Score          float64                  `json:"score"`
	Category       RiskCategory             `json:"category"`
	Recommendation string                   `json:"recommendation"`
	ComputedAt     time.Time                `json:"computedAt"`
	Factors        []map[string]interface{} `json:"factors,omitempty"`
}

// Discrepancy — расхождение из reconciliation.
type Discrepancy struct {
	Field      string `json:"field"`
	DocumentID string `json:"documentId,omitempty"`
	Expected   string `json:"expected,omitempty"`
	Actual     string `json:"actual,omitempty"`
	Severity   string `json:"severity"`
}

// Decision — финальное решение.
type Decision struct {
	ID            string       `json:"id"`
	ApplicationID string       `json:"applicationId"`
	Decision      DecisionKind `json:"decision"`
	Reasoning     string       `json:"reasoning"`
	DecidedAt     time.Time    `json:"decidedAt"`
}

// Application — главная сущность.
type Application struct {
	ID              string           `json:"id"`
	TenantID        string           `json:"tenantId"`
	State           ApplicationState `json:"state"`
	LegalEntityType string           `json:"legalEntityType"`
	Channel         string           `json:"channel"`
	ProductCodes    []string         `json:"productCodes"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	ApplicantID     string           `json:"applicantId,omitempty"`
}

// SubmitApplicationInput — DTO мутации submitApplication.
type SubmitApplicationInput struct {
	LegalEntityType string   `json:"legalEntityType"`
	Channel         string   `json:"channel"`
	ProductCodes    []string `json:"productCodes"`
}

// UploadDocumentInput — DTO мутации uploadDocument.
type UploadDocumentInput struct {
	ApplicationID string       `json:"applicationId"`
	Type          DocumentType `json:"type"`
	Filename      string       `json:"filename"`
	ContentBase64 string       `json:"contentBase64"`
}

// Package domainmodel defines the canonical domain model for the AIbank
// white-label digital onboarding platform.
//
// This file is hand-written and is the authoritative Go representation of
// schema.json. It must be kept in sync with the JSON Schema manually.
//
// ID format: {prefix}_{ULID}  e.g. tnt_01ARZ3NDEKTSV4RRFFQ69G5FAV
// Money:     always int64 (kopecks), never float
// Dates:     UTC ISO 8601 strings
package domainmodel

// ---------------------------------------------------------------------------
// Shared primitive types (named for documentation clarity)
// ---------------------------------------------------------------------------

// ApplicationStatus is the state-machine status of an onboarding Application.
type ApplicationStatus string

const (
	ApplicationStatusDraft               ApplicationStatus = "draft"
	ApplicationStatusDocumentsPending    ApplicationStatus = "documents_pending"
	ApplicationStatusValidationInProgress ApplicationStatus = "validation_in_progress"
	ApplicationStatusRiskScoring         ApplicationStatus = "risk_scoring"
	ApplicationStatusManualReview        ApplicationStatus = "manual_review"
	ApplicationStatusAutoApproved        ApplicationStatus = "auto_approved"
	ApplicationStatusRejected            ApplicationStatus = "rejected"
	ApplicationStatusAccountOpening      ApplicationStatus = "account_opening"
	ApplicationStatusCompleted           ApplicationStatus = "completed"
	ApplicationStatusCancelled           ApplicationStatus = "cancelled"
)

// DocumentType is the semantic category of an uploaded document.
type DocumentType string

const (
	DocumentTypePassport                DocumentType = "passport"
	DocumentTypeInnCertificate          DocumentType = "inn_certificate"
	DocumentTypeOgrnCertificate         DocumentType = "ogrn_certificate"
	DocumentTypeCharter                 DocumentType = "charter"
	DocumentTypeBeneficialOwnersRegistry DocumentType = "beneficial_owners_registry"
	DocumentTypeCeoAppointmentOrder     DocumentType = "ceo_appointment_order"
	DocumentTypeBankAccountApplication  DocumentType = "bank_account_application"
	DocumentTypePowerOfAttorney         DocumentType = "power_of_attorney"
	DocumentTypeFinancialStatements     DocumentType = "financial_statements"
	DocumentTypeOther                   DocumentType = "other"
)

// DocumentStatus is the processing state of a document.
type DocumentStatus string

const (
	DocumentStatusUploaded   DocumentStatus = "uploaded"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusVerified   DocumentStatus = "verified"
	DocumentStatusRejected   DocumentStatus = "rejected"
)

// LegalForm is the legal organisational form of a Russian entity.
type LegalForm string

const (
	LegalFormOOO LegalForm = "ooo"
	LegalFormAO  LegalForm = "ao"
	LegalFormPAO LegalForm = "pao"
	LegalFormIP  LegalForm = "ip"
	LegalFormZAO LegalForm = "zao"
)

// LegalEntityStatus is the lifecycle status from the state register.
type LegalEntityStatus string

const (
	LegalEntityStatusActive      LegalEntityStatus = "active"
	LegalEntityStatusLiquidating LegalEntityStatus = "liquidating"
	LegalEntityStatusLiquidated  LegalEntityStatus = "liquidated"
	LegalEntityStatusReorganizing LegalEntityStatus = "reorganizing"
)

// RiskLevel is the bucketed risk classification.
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// Recommendation is the model recommendation based on risk score.
type Recommendation string

const (
	RecommendationApprove      Recommendation = "approve"
	RecommendationManualReview Recommendation = "manual_review"
	RecommendationReject       Recommendation = "reject"
)

// DecisionType describes how and in which direction a decision was made.
type DecisionType string

const (
	DecisionTypeAutoApproved    DecisionType = "auto_approved"
	DecisionTypeAutoRejected    DecisionType = "auto_rejected"
	DecisionTypeManualApproved  DecisionType = "manual_approved"
	DecisionTypeManualRejected  DecisionType = "manual_rejected"
	DecisionTypeEscalated       DecisionType = "escalated"
)

// Currency is an ISO 4217 currency code.
type Currency string

const (
	CurrencyRUB Currency = "RUB"
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

// AccountType is the functional type of a bank account.
type AccountType string

const (
	AccountTypeSettlement AccountType = "settlement"
	AccountTypeDeposit    AccountType = "deposit"
	AccountTypeLoan       AccountType = "loan"
)

// SignatureType is the type of electronic signature on a document.
type SignatureType string

const (
	SignatureTypeUKEP SignatureType = "ukep"
	SignatureTypePEP  SignatureType = "pep"
	SignatureTypeNone SignatureType = "none"
)

// UBONodeType is the type of a node in the beneficial ownership graph.
type UBONodeType string

const (
	UBONodeTypePerson      UBONodeType = "person"
	UBONodeTypeLegalEntity UBONodeType = "legal_entity"
)

// ---------------------------------------------------------------------------
// Sub-entities / value objects
// ---------------------------------------------------------------------------

// RiskFactor is a single factor contributing to a risk score.
type RiskFactor struct {
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	Impact      int         `json:"impact"`
	Explanation string      `json:"explanation"`
}

// UBONode is a node (person or legal entity) in the beneficial ownership graph.
type UBONode struct {
	ID             string      `json:"id"`
	PersonID       *string     `json:"person_id,omitempty"`
	LegalEntityID  *string     `json:"legal_entity_id,omitempty"`
	Name           string      `json:"name"`
	NodeType       UBONodeType `json:"node_type"`
	DirectStake    float64     `json:"direct_stake"`
	EffectiveStake float64     `json:"effective_stake"`
	IsUBO          bool        `json:"is_ubo"`
}

// UBOEdge is a directed ownership edge between two nodes in the beneficial
// ownership graph.
type UBOEdge struct {
	FromNodeID string  `json:"from_node_id"`
	ToNodeID   string  `json:"to_node_id"`
	Stake      float64 `json:"stake"`
	DocumentID *string `json:"document_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Core entities
// ---------------------------------------------------------------------------

// Tenant represents a bank that uses the AIbank platform. It is the root
// multi-tenant entity — every other entity belongs to exactly one Tenant.
type Tenant struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Status        string `json:"status"`
	ConfigVersion string `json:"config_version"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Application is a single onboarding application. Core state-machine entity.
type Application struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	ClientID      *string           `json:"client_id,omitempty"`
	LegalEntityID *string           `json:"legal_entity_id,omitempty"`
	Status        ApplicationStatus `json:"status"`
	WorkflowID    *string           `json:"workflow_id,omitempty"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
	SubmittedAt   *string           `json:"submitted_at,omitempty"`
	DecidedAt     *string           `json:"decided_at,omitempty"`
	AccountID     *string           `json:"account_id,omitempty"`
}

// Person is any natural person in the system: applicant, director, founder,
// UBO, or signatory.
type Person struct {
	ID                 string  `json:"id"`
	TenantID           string  `json:"tenant_id"`
	LastName           string  `json:"last_name"`
	FirstName          string  `json:"first_name"`
	MiddleName         *string `json:"middle_name,omitempty"`
	BirthDate          string  `json:"birth_date"`
	INN                string  `json:"inn"`
	SNILS              *string `json:"snils,omitempty"`
	PassportSeries     string  `json:"passport_series"`
	PassportNumber     string  `json:"passport_number"`
	PassportIssuedBy   string  `json:"passport_issued_by"`
	PassportIssuedAt   string  `json:"passport_issued_at"`
	CreatedAt          string  `json:"created_at"`
}

// LegalEntity is a Russian legal entity (ООО, АО, etc.) undergoing or having
// completed onboarding.
type LegalEntity struct {
	ID               string            `json:"id"`
	TenantID         string            `json:"tenant_id"`
	FullName         string            `json:"full_name"`
	ShortName        string            `json:"short_name"`
	INN              string            `json:"inn"`
	OGRN             string            `json:"ogrn"`
	KPP              *string           `json:"kpp,omitempty"`
	LegalForm        LegalForm         `json:"legal_form"`
	LegalAddress     string            `json:"legal_address"`
	ActualAddress    *string           `json:"actual_address,omitempty"`
	OKVEDPrimary     string            `json:"okved_primary"`
	OKVEDSecondary   []string          `json:"okved_secondary"`
	RegistrationDate string            `json:"registration_date"`
	Status           LegalEntityStatus `json:"status"`
	CEOPersonID      string            `json:"ceo_person_id"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

// UBOGraph is the computed beneficial ownership graph for one legal entity in
// the context of an application.
type UBOGraph struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	ApplicationID string    `json:"application_id"`
	LegalEntityID string    `json:"legal_entity_id"`
	Nodes         []UBONode `json:"nodes"`
	Edges         []UBOEdge `json:"edges"`
	ComputedAt    string    `json:"computed_at"`
}

// Document is a document uploaded by the applicant or retrieved from an
// external source.
type Document struct {
	ID               string                 `json:"id"`
	TenantID         string                 `json:"tenant_id"`
	ApplicationID    string                 `json:"application_id"`
	DocumentType     DocumentType           `json:"document_type"`
	Filename         string                 `json:"filename"`
	MIMEType         string                 `json:"mime_type"`
	SizeBytes        int64                  `json:"size_bytes"`
	S3Key            string                 `json:"s3_key"`
	ChecksumSHA256   string                 `json:"checksum_sha256"`
	Status           DocumentStatus         `json:"status"`
	ExtractedFields  map[string]interface{} `json:"extracted_fields,omitempty"`
	OCRConfidence    *float64               `json:"ocr_confidence,omitempty"`
	SignedAt         *string                `json:"signed_at,omitempty"`
	SignatureType    SignatureType           `json:"signature_type"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

// RiskAssessment is the ML-based risk scoring result for an onboarding
// application.
type RiskAssessment struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ApplicationID  string         `json:"application_id"`
	Score          int            `json:"score"`
	RiskLevel      RiskLevel      `json:"risk_level"`
	Recommendation Recommendation `json:"recommendation"`
	Factors        []RiskFactor   `json:"factors"`
	ModelVersion   string         `json:"model_version"`
	AssessedAt     string         `json:"assessed_at"`
}

// Decision is the final underwriting decision on an onboarding application.
type Decision struct {
	ID               string       `json:"id"`
	TenantID         string       `json:"tenant_id"`
	ApplicationID    string       `json:"application_id"`
	DecisionType     DecisionType `json:"decision_type"`
	ActorID          *string      `json:"actor_id,omitempty"`
	Reason           string       `json:"reason"`
	RiskAssessmentID string       `json:"risk_assessment_id"`
	CreatedAt        string       `json:"created_at"`
}

// Account is a bank account opened for the legal entity after an approved
// application.
type Account struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant_id"`
	ApplicationID string      `json:"application_id"`
	LegalEntityID string      `json:"legal_entity_id"`
	AccountNumber string      `json:"account_number"`
	BIK           string      `json:"bik"`
	BankName      string      `json:"bank_name"`
	Currency      Currency    `json:"currency"`
	AccountType   AccountType `json:"account_type"`
	OpenedAt      string      `json:"opened_at"`
	ABSReference  *string     `json:"abs_reference,omitempty"`
	CreatedAt     string      `json:"created_at"`
}

// AuditEvent is an immutable append-only audit log entry.
type AuditEvent struct {
	ID           string                 `json:"id"`
	TenantID     string                 `json:"tenant_id"`
	EventType    string                 `json:"event_type"`
	ActorID      string                 `json:"actor_id"`
	ActorRole    string                 `json:"actor_role"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id"`
	Payload      map[string]interface{} `json:"payload"`
	IPAddress    *string                `json:"ip_address,omitempty"`
	CreatedAt    string                 `json:"created_at"`
}

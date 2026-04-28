// Code generated from schema.json. DO NOT EDIT.
// Source: schema.json (https://aibank.io/schemas/domain-model/v1.0.0/schema.json)

package domain

import "time"

// ApplicationStatus State machine status for an onboarding application
type ApplicationStatus string

const (
	ApplicationStatusDraft ApplicationStatus = "draft"
	ApplicationStatusDocumentsPending ApplicationStatus = "documents_pending"
	ApplicationStatusValidationInProgress ApplicationStatus = "validation_in_progress"
	ApplicationStatusRiskScoring ApplicationStatus = "risk_scoring"
	ApplicationStatusManualReview ApplicationStatus = "manual_review"
	ApplicationStatusAutoApproved ApplicationStatus = "auto_approved"
	ApplicationStatusRejected ApplicationStatus = "rejected"
	ApplicationStatusAccountOpening ApplicationStatus = "account_opening"
	ApplicationStatusCompleted ApplicationStatus = "completed"
	ApplicationStatusCancelled ApplicationStatus = "cancelled"
)

// DocumentType Semantic category of an uploaded document
type DocumentType string

const (
	DocumentTypePassport DocumentType = "passport"
	DocumentTypeInnCertificate DocumentType = "inn_certificate"
	DocumentTypeOgrnCertificate DocumentType = "ogrn_certificate"
	DocumentTypeCharter DocumentType = "charter"
	DocumentTypeBeneficialOwnersRegistry DocumentType = "beneficial_owners_registry"
	DocumentTypeCeoAppointmentOrder DocumentType = "ceo_appointment_order"
	DocumentTypeBankAccountApplication DocumentType = "bank_account_application"
	DocumentTypePowerOfAttorney DocumentType = "power_of_attorney"
	DocumentTypeFinancialStatements DocumentType = "financial_statements"
	DocumentTypeOther DocumentType = "other"
)

// DocumentStatus Processing state of a document
type DocumentStatus string

const (
	DocumentStatusUploaded DocumentStatus = "uploaded"
	DocumentStatusProcessing DocumentStatus = "processing"
	DocumentStatusVerified DocumentStatus = "verified"
	DocumentStatusRejected DocumentStatus = "rejected"
)

// LegalForm Legal organisational form of a Russian entity
type LegalForm string

const (
	LegalFormOoo LegalForm = "ooo"
	LegalFormAo LegalForm = "ao"
	LegalFormPao LegalForm = "pao"
	LegalFormIp LegalForm = "ip"
	LegalFormZao LegalForm = "zao"
)

// LegalEntityStatus Lifecycle status of a legal entity from the state register
type LegalEntityStatus string

const (
	LegalEntityStatusActive LegalEntityStatus = "active"
	LegalEntityStatusLiquidating LegalEntityStatus = "liquidating"
	LegalEntityStatusLiquidated LegalEntityStatus = "liquidated"
	LegalEntityStatusReorganizing LegalEntityStatus = "reorganizing"
)

// RiskLevel Bucketed risk classification
type RiskLevel string

const (
	RiskLevelLow RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// Recommendation Model recommendation based on risk score
type Recommendation string

const (
	RecommendationApprove Recommendation = "approve"
	RecommendationManualReview Recommendation = "manual_review"
	RecommendationReject Recommendation = "reject"
)

// DecisionType How and in which direction the decision was made
type DecisionType string

const (
	DecisionTypeAutoApproved DecisionType = "auto_approved"
	DecisionTypeAutoRejected DecisionType = "auto_rejected"
	DecisionTypeManualApproved DecisionType = "manual_approved"
	DecisionTypeManualRejected DecisionType = "manual_rejected"
	DecisionTypeEscalated DecisionType = "escalated"
)

// Currency ISO 4217 currency code
type Currency string

const (
	CurrencyRub Currency = "RUB"
	CurrencyUsd Currency = "USD"
	CurrencyEur Currency = "EUR"
)

// AccountType Functional type of a bank account
type AccountType string

const (
	AccountTypeSettlement AccountType = "settlement"
	AccountTypeDeposit AccountType = "deposit"
	AccountTypeLoan AccountType = "loan"
)

// SignatureType Type of electronic signature on a document
type SignatureType string

const (
	SignatureTypeUkep SignatureType = "ukep"
	SignatureTypePep SignatureType = "pep"
	SignatureTypeNone SignatureType = "none"
)

// UBONodeType Type of node in the beneficial ownership graph
type UBONodeType string

const (
	UBONodeTypePerson UBONodeType = "person"
	UBONodeTypeLegalEntity UBONodeType = "legal_entity"
)

// RiskFactor A single factor contributing to a risk score
type RiskFactor struct {
	// Name Machine-readable factor identifier
	Name string `json:"name"`
	// Value Observed value of the factor (any scalar)
	Value interface{} `json:"value"`
	// Impact Signed integer contribution to the overall score (positive = higher risk)
	Impact int64 `json:"impact"`
	// Explanation Human-readable explanation of how this factor affects the score
	Explanation string `json:"explanation"`
}

// UBONode A node (person or legal entity) in the beneficial ownership graph
type UBONode struct {
	// ID Node identifier within the graph
	ID string `json:"id"`
	// PersonID Reference to Person entity — present when node_type is person
	PersonID *string `json:"person_id,omitempty"`
	// LegalEntityID Reference to LegalEntity — present when node_type is legal_entity
	LegalEntityID *string `json:"legal_entity_id,omitempty"`
	// Name Display name of the node
	Name string `json:"name"`
	NodeType UBONodeType `json:"node_type"`
	// DirectStake Direct ownership stake as a percentage (0–100)
	DirectStake float64 `json:"direct_stake"`
	// EffectiveStake Computed effective (indirect + direct) ownership stake as a percentage
	EffectiveStake float64 `json:"effective_stake"`
	// IsUBO True when effective_stake >= 25% (ultimate beneficial owner threshold)
	IsUBO bool `json:"is_ubo"`
}

// UBOEdge A directed ownership edge between two nodes in the beneficial ownership graph
type UBOEdge struct {
	// FromNodeID Source node identifier
	FromNodeID string `json:"from_node_id"`
	// ToNodeID Target node identifier
	ToNodeID string `json:"to_node_id"`
	// Stake Ownership stake on this edge as a percentage
	Stake float64 `json:"stake"`
	// DocumentID Supporting document that evidences this ownership stake
	DocumentID *string `json:"document_id,omitempty"`
}

// Tenant A bank that uses the AIbank platform. Root multi-tenant entity — every other entity belongs to exactly one Tenant.
type Tenant struct {
	ID string `json:"id"`
	// Name Display name, e.g. 'Альфа-Банк'
	Name string `json:"name"`
	// Slug Kebab-case unique identifier used in URLs and config paths
	Slug string `json:"slug"`
	// Status Lifecycle status of the tenant account
	Status string `json:"status"`
	// ConfigVersion Semver of the tenant configuration currently applied
	ConfigVersion string `json:"config_version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Application A single onboarding application. Core state-machine entity — one per legal entity from draft to account opening or rejection.
type Application struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	// ClientID External client reference (nullable)
	ClientID *string `json:"client_id,omitempty"`
	// LegalEntityID Legal entity under review (nullable until entity is created)
	LegalEntityID *string `json:"legal_entity_id,omitempty"`
	Status ApplicationStatus `json:"status"`
	// WorkflowID Temporal workflow execution ID tracking this application
	WorkflowID *string `json:"workflow_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// SubmittedAt When the applicant submitted the application (left draft state)
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	// DecidedAt When a final approve/reject decision was recorded
	DecidedAt *time.Time `json:"decided_at,omitempty"`
	// AccountID Opened account — null until status is completed
	AccountID *string `json:"account_id,omitempty"`
}

// Person Any natural person in the system: applicant, director, founder, UBO, signatory.
type Person struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	LastName string `json:"last_name"`
	FirstName string `json:"first_name"`
	// MiddleName Patronymic (отчество), optional
	MiddleName *string `json:"middle_name,omitempty"`
	// BirthDate Date of birth, UTC ISO 8601
	BirthDate time.Time `json:"birth_date"`
	INN string `json:"inn"`
	// Snils Insurance number — optional
	Snils *string `json:"snils,omitempty"`
	// PassportSeries Russian passport series (4 digits)
	PassportSeries string `json:"passport_series"`
	// PassportNumber Russian passport number (6 digits)
	PassportNumber string `json:"passport_number"`
	// PassportIssuedBy Name of the issuing authority
	PassportIssuedBy string `json:"passport_issued_by"`
	// PassportIssuedAt Date the passport was issued, UTC ISO 8601
	PassportIssuedAt time.Time `json:"passport_issued_at"`
	CreatedAt time.Time `json:"created_at"`
}

// LegalEntity A Russian legal entity (ООО, АО, etc.) undergoing or having completed onboarding.
type LegalEntity struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	// FullName Full official name from EGRUL
	FullName string `json:"full_name"`
	// ShortName Abbreviated name, e.g. 'ООО «Ромашка»'
	ShortName string `json:"short_name"`
	INN string `json:"inn"`
	OGRN string `json:"ogrn"`
	// KPP KPP — nullable for individual entrepreneurs (IP)
	KPP *string `json:"kpp,omitempty"`
	LegalForm LegalForm `json:"legal_form"`
	// LegalAddress Registered legal address as plain text
	LegalAddress string `json:"legal_address"`
	// ActualAddress Actual operating address — nullable if same as legal_address
	ActualAddress *string `json:"actual_address,omitempty"`
	// OkvedPrimary Primary OKVED activity code
	OkvedPrimary string `json:"okved_primary"`
	// OkvedSecondary Additional OKVED activity codes
	OkvedSecondary []string `json:"okved_secondary"`
	// RegistrationDate Date of state registration
	RegistrationDate time.Time `json:"registration_date"`
	Status LegalEntityStatus `json:"status"`
	// CeoPersonID Reference to the Person who is the current CEO/director
	CeoPersonID string `json:"ceo_person_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UBOGraph Computed beneficial ownership graph for one legal entity in the context of an application.
type UBOGraph struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	LegalEntityID string `json:"legal_entity_id"`
	// Nodes All persons and entities in the ownership chain
	Nodes []UBONode `json:"nodes"`
	// Edges Directed ownership edges between nodes
	Edges []UBOEdge `json:"edges"`
	ComputedAt time.Time `json:"computed_at"`
}

// Document A document uploaded by the applicant or retrieved from an external source.
type Document struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	DocumentType DocumentType `json:"document_type"`
	// Filename Original filename as provided by the uploader
	Filename string `json:"filename"`
	// MimeType MIME type, e.g. 'application/pdf', 'image/jpeg'
	MimeType string `json:"mime_type"`
	// SizeBytes File size in bytes
	SizeBytes int64 `json:"size_bytes"`
	// S3Key Object storage key, e.g. 'tnt_xxx/app_yyy/doc_zzz/passport.pdf'
	S3Key string `json:"s3_key"`
	// ChecksumSha256 Hex-encoded SHA-256 checksum of the file content
	ChecksumSha256 string `json:"checksum_sha256"`
	Status DocumentStatus `json:"status"`
	// ExtractedFields Key-value pairs extracted by OCR/ML — schema varies by document_type
	ExtractedFields map[string]interface{} `json:"extracted_fields,omitempty"`
	// OCRConfidence Overall OCR confidence score (0–1); null when not yet processed
	OCRConfidence *float64 `json:"ocr_confidence,omitempty"`
	// SignedAt When the document was electronically signed — null if unsigned
	SignedAt *time.Time `json:"signed_at,omitempty"`
	SignatureType SignatureType `json:"signature_type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RiskAssessment ML-based risk scoring result for an onboarding application.
type RiskAssessment struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	// Score Risk score from 0 (lowest risk) to 100 (highest risk)
	Score int64 `json:"score"`
	RiskLevel RiskLevel `json:"risk_level"`
	Recommendation Recommendation `json:"recommendation"`
	// Factors Ordered list of factors that contributed to the score
	Factors []RiskFactor `json:"factors"`
	// ModelVersion Semver of the risk model that produced this assessment
	ModelVersion string `json:"model_version"`
	AssessedAt time.Time `json:"assessed_at"`
}

// Decision The final underwriting decision on an onboarding application.
type Decision struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	DecisionType DecisionType `json:"decision_type"`
	// ActorID User ID of the operator for manual decisions; null for automated decisions
	ActorID *string `json:"actor_id,omitempty"`
	// Reason Human-readable explanation — mandatory for rejections
	Reason string `json:"reason"`
	// RiskAssessmentID The risk assessment that informed this decision
	RiskAssessmentID string `json:"risk_assessment_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Account A bank account opened for the legal entity after an approved application.
type Account struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	LegalEntityID string `json:"legal_entity_id"`
	AccountNumber string `json:"account_number"`
	BIK string `json:"bik"`
	// BankName Name of the bank holding the account
	BankName string `json:"bank_name"`
	Currency Currency `json:"currency"`
	AccountType AccountType `json:"account_type"`
	OpenedAt time.Time `json:"opened_at"`
	// AbsReference Internal reference identifier in the bank's core banking system (АБС)
	AbsReference *string `json:"abs_reference,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEvent Immutable append-only audit log entry.
type AuditEvent struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	// EventType Dot-namespaced event name, e.g. 'application.submitted', 'document.uploaded'
	EventType string `json:"event_type"`
	// ActorID Identifier of the actor: a user ID, 'system', or an AI agent name
	ActorID string `json:"actor_id"`
	// ActorRole Role of the actor, e.g. 'operator', 'system', 'ai_agent'
	ActorRole string `json:"actor_role"`
	// ResourceType Domain entity type that was affected, e.g. 'application', 'document', 'decision'
	ResourceType string `json:"resource_type"`
	// ResourceID Identifier of the affected entity
	ResourceID string `json:"resource_id"`
	// Payload Domain-specific event payload; schema varies by event_type
	Payload map[string]interface{} `json:"payload"`
	// IPAddress IP address of the request origin — null for system-generated events
	IPAddress *string `json:"ip_address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

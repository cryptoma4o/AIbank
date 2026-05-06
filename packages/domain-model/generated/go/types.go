// Code generated from schema.json. DO NOT EDIT.
// Source: schema.json (https://aibank.io/schemas/domain-model/v1.1.0/schema.json)

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

// DocumentType Semantic category of an uploaded document. Расширено в v1.1.0 для покрытия этапа 6 формы (см. docs/onboarding-form-spec.md). Add-only: старые значения сохранены.
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
	DocumentTypeEgrulRecordSheet DocumentType = "egrul_record_sheet"
	DocumentTypeEioAppointmentProtocol DocumentType = "eio_appointment_protocol"
	DocumentTypeEioOrder DocumentType = "eio_order"
	DocumentTypeSignatureCard DocumentType = "signature_card"
	DocumentTypeLicense DocumentType = "license"
	DocumentTypeAddressConfirmation DocumentType = "address_confirmation"
	DocumentTypeTaxDeclaration DocumentType = "tax_declaration"
	DocumentTypeTaxClearanceCertificate DocumentType = "tax_clearance_certificate"
	DocumentTypeOwnershipChainDiagram DocumentType = "ownership_chain_diagram"
	DocumentTypeFatcaW8Form DocumentType = "fatca_w8_form"
	DocumentTypeFatcaW9Form DocumentType = "fatca_w9_form"
	DocumentTypeMigrationCard DocumentType = "migration_card"
	DocumentTypeResidencePermit DocumentType = "residence_permit"
	DocumentTypeForeignPassport DocumentType = "foreign_passport"
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

// AccountType Functional type of a bank account. Add-only: расширено в v1.1.0 для покрытия этапа 9 (settlement/special/foreign_currency/escrow/nominal — см. docs/onboarding-form-spec.md §9).
type AccountType string

const (
	AccountTypeSettlement AccountType = "settlement"
	AccountTypeDeposit AccountType = "deposit"
	AccountTypeLoan AccountType = "loan"
	AccountTypeSpecial AccountType = "special"
	AccountTypeForeignCurrency AccountType = "foreign_currency"
	AccountTypeEscrow AccountType = "escrow"
	AccountTypeNominal AccountType = "nominal"
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

// TaxRegime Tax regime applicable to a Russian legal entity / IP
type TaxRegime string

const (
	TaxRegimeOsn TaxRegime = "osn"
	TaxRegimeUsnIncome TaxRegime = "usn_income"
	TaxRegimeUsnIncomeMinusExpenses TaxRegime = "usn_income_minus_expenses"
	TaxRegimePsn TaxRegime = "psn"
	TaxRegimeEshn TaxRegime = "eshn"
	TaxRegimeNpd TaxRegime = "npd"
)

// AuthorityBasis Legal basis on which the representative acts on behalf of the legal entity (этап 4)
type AuthorityBasis string

const (
	AuthorityBasisCharter AuthorityBasis = "charter"
	AuthorityBasisProtocol AuthorityBasis = "protocol"
	AuthorityBasisPowerOfAttorney AuthorityBasis = "power_of_attorney"
	AuthorityBasisOrder AuthorityBasis = "order"
)

// IDDocumentType Type of identity document presented by a representative / UBO
type IDDocumentType string

const (
	IDDocumentTypePassportRu IDDocumentType = "passport_ru"
	IDDocumentTypePassportForeign IDDocumentType = "passport_foreign"
	IDDocumentTypeNationalPassport IDDocumentType = "national_passport"
	IDDocumentTypeRefugeeCertificate IDDocumentType = "refugee_certificate"
)

// PDLCategory Politically-exposed-person category — uses PEP terminology in regulatory texts
type PDLCategory string

const (
	PDLCategoryForeign PDLCategory = "foreign"
	PDLCategoryRussian PDLCategory = "russian"
	PDLCategoryInternationalOrganization PDLCategory = "international_organization"
)

// PDLRelation How the person relates to a politically-exposed-person status
type PDLRelation string

const (
	PDLRelationSelf PDLRelation = "self"
	PDLRelationRelative PDLRelation = "relative"
	PDLRelationRepresentative PDLRelation = "representative"
)

// BusinessRiskCategory AML risk category of the declared OKVED activity (этап 3)
type BusinessRiskCategory string

const (
	BusinessRiskCategoryLowRisk BusinessRiskCategory = "low_risk"
	BusinessRiskCategoryMediumRisk BusinessRiskCategory = "medium_risk"
	BusinessRiskCategoryHighRisk BusinessRiskCategory = "high_risk"
)

// FundsSourceCategory Declared origin of operating funds
type FundsSourceCategory string

const (
	FundsSourceCategoryRevenue FundsSourceCategory = "revenue"
	FundsSourceCategoryFounderContribution FundsSourceCategory = "founder_contribution"
	FundsSourceCategoryLoan FundsSourceCategory = "loan"
	FundsSourceCategoryInvestments FundsSourceCategory = "investments"
	FundsSourceCategoryOther FundsSourceCategory = "other"
)

// ControlBasis Basis on which a person controls the legal entity (UBO — этап 5)
type ControlBasis string

const (
	ControlBasisCapitalShare ControlBasis = "capital_share"
	ControlBasisContract ControlBasis = "contract"
	ControlBasisOtherDecisionRight ControlBasis = "other_decision_right"
)

// ScreeningMatchLevel Screening match outcome against a watch list
type ScreeningMatchLevel string

const (
	ScreeningMatchLevelMatch ScreeningMatchLevel = "match"
	ScreeningMatchLevelPartialMatch ScreeningMatchLevel = "partial_match"
	ScreeningMatchLevelNoMatch ScreeningMatchLevel = "no_match"
)

// AdverseMediaCategory Category of adverse media hit
type AdverseMediaCategory string

const (
	AdverseMediaCategoryFraud AdverseMediaCategory = "fraud"
	AdverseMediaCategoryMoneyLaundering AdverseMediaCategory = "money_laundering"
	AdverseMediaCategoryTerrorismFinancing AdverseMediaCategory = "terrorism_financing"
	AdverseMediaCategorySanctions AdverseMediaCategory = "sanctions"
	AdverseMediaCategoryCorruption AdverseMediaCategory = "corruption"
	AdverseMediaCategoryOther AdverseMediaCategory = "other"
)

// RelationshipType Counterparty relationship — постоянный / разовый
type RelationshipType string

const (
	RelationshipTypeRegular RelationshipType = "regular"
	RelationshipTypeOneOff RelationshipType = "one_off"
)

// KYCRefreshTrigger Event that triggers re-execution of KYC procedures
type KYCRefreshTrigger string

const (
	KYCRefreshTriggerScheduled KYCRefreshTrigger = "scheduled"
	KYCRefreshTriggerCeoChange KYCRefreshTrigger = "ceo_change"
	KYCRefreshTriggerUboChange KYCRefreshTrigger = "ubo_change"
	KYCRefreshTriggerOwnershipChange KYCRefreshTrigger = "ownership_change"
	KYCRefreshTriggerAddressChange KYCRefreshTrigger = "address_change"
	KYCRefreshTriggerLicenseExpiry KYCRefreshTrigger = "license_expiry"
	KYCRefreshTriggerTransactionThresholdExceeded KYCRefreshTrigger = "transaction_threshold_exceeded"
	KYCRefreshTriggerRegulatorAlert KYCRefreshTrigger = "regulator_alert"
)

// SigningMethod Method used to sign account-opening agreements
type SigningMethod string

const (
	SigningMethodUkep SigningMethod = "ukep"
	SigningMethodSmsCode SigningMethod = "sms_code"
	SigningMethodHandwritten SigningMethod = "handwritten"
)

// RemoteBankingChannel Channel through which DBO (remote banking) is accessed
type RemoteBankingChannel string

const (
	RemoteBankingChannelWeb RemoteBankingChannel = "web"
	RemoteBankingChannelMobile RemoteBankingChannel = "mobile"
	RemoteBankingChannelApi RemoteBankingChannel = "api"
)

// ZSKLight ЗСК (Знай своего клиента) traffic-light category from CBR
type ZSKLight string

const (
	ZSKLightGreen ZSKLight = "green"
	ZSKLightYellow ZSKLight = "yellow"
	ZSKLightRed ZSKLight = "red"
	ZSKLightUnknown ZSKLight = "unknown"
)

// PrequalificationDecision Aggregated decision after the pre-qualification stage (этап 1)
type PrequalificationDecision string

const (
	PrequalificationDecisionProceed PrequalificationDecision = "proceed"
	PrequalificationDecisionManualReview PrequalificationDecision = "manual_review"
	PrequalificationDecisionReject PrequalificationDecision = "reject"
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

// StructuredAddress Structured address per ФИАС, used in legal/actual/postal address blocks (этап 2) and для физлица (этап 4).
type StructuredAddress struct {
	CountryCode string `json:"country_code"`
	// PostalCode Postal index (free-form to support non-RU formats)
	PostalCode *string `json:"postal_code,omitempty"`
	// RegionCode ФИАС region code — applicable for RU
	RegionCode *string `json:"region_code,omitempty"`
	// RegionName Region / state / province name
	RegionName *string `json:"region_name,omitempty"`
	// City City / locality
	City string `json:"city"`
	// Street Street name
	Street *string `json:"street,omitempty"`
	// Building Building / house number
	Building *string `json:"building,omitempty"`
	// Office Apartment / office / floor
	Office *string `json:"office,omitempty"`
	// FiasID ФИАС GUID, if resolved
	FiasID *string `json:"fias_id,omitempty"`
}

// MoneyAmount Monetary amount in a specific currency
type MoneyAmount struct {
	// Amount Decimal amount (precision should match currency convention)
	Amount float64 `json:"amount"`
	Currency string `json:"currency"`
}

// ContactInfo Contact details for a legal entity (этап 2)
type ContactInfo struct {
	Phone *string `json:"phone,omitempty"`
	Email *string `json:"email,omitempty"`
	Website *string `json:"website,omitempty"`
}

// LicenseInfo Licence held by a legal entity (этап 2)
type LicenseInfo struct {
	Number string `json:"number"`
	IssueDate string `json:"issue_date"`
	ExpiryDate *string `json:"expiry_date,omitempty"`
	Issuer string `json:"issuer"`
	ActivityType string `json:"activity_type"`
}

// SROMembership Self-regulating organisation membership (этап 2)
type SROMembership struct {
	Name string `json:"name"`
	RegNumber string `json:"reg_number"`
	JoinDate string `json:"join_date"`
}

// Counterparty Top supplier or top buyer counterparty (этап 3)
type Counterparty struct {
	Name string `json:"name"`
	INN *string `json:"inn,omitempty"`
	Country string `json:"country"`
	SharePercent float64 `json:"share_percent"`
	RelationshipType RelationshipType `json:"relationship_type"`
}

// OperationalModel Planned operational model (этап 3)
type OperationalModel struct {
	Geography []string `json:"geography,omitempty"`
	MonthlyTurnoverPlanned *MoneyAmount `json:"monthly_turnover_planned,omitempty"`
	AnnualTurnoverPlanned *MoneyAmount `json:"annual_turnover_planned,omitempty"`
	CashSharePercent *float64 `json:"cash_share_percent,omitempty"`
	ForeignEconomicActivity *bool `json:"foreign_economic_activity,omitempty"`
	ForeignCountries []string `json:"foreign_countries,omitempty"`
	CurrencyOperations []string `json:"currency_operations,omitempty"`
}

// FundsSource Declared source of funds (этап 3)
type FundsSource struct {
	Category FundsSourceCategory `json:"category"`
	// Description Free-form justification — required when category=other
	Description *string `json:"description,omitempty"`
}

// IDDocument Identity document for a representative or UBO (этап 4/5)
type IDDocument struct {
	DocType IDDocumentType `json:"doc_type"`
	// Series RU passport series (4 digits) — null for non-RU
	Series *string `json:"series,omitempty"`
	// Number Document number (formats vary)
	Number string `json:"number"`
	IssueDate *string `json:"issue_date,omitempty"`
	ExpiryDate *string `json:"expiry_date,omitempty"`
	IssuedBy *string `json:"issued_by,omitempty"`
	// DepartmentCode RU department code (XXX-XXX) — null for non-RU
	DepartmentCode *string `json:"department_code,omitempty"`
}

// AuthorityInfo Powers of a representative (этап 4)
type AuthorityInfo struct {
	Position string `json:"position"`
	AuthorityBasis AuthorityBasis `json:"authority_basis"`
	AuthorityDocNumber *string `json:"authority_doc_number,omitempty"`
	AuthorityDocDate *string `json:"authority_doc_date,omitempty"`
	SignatureSampleDocID *string `json:"signature_sample_doc_id,omitempty"`
}

// ForeignerInfo Additional details required for non-RU citizens (этап 4)
type ForeignerInfo struct {
	MigrationCardNumber *string `json:"migration_card_number,omitempty"`
	MigrationCardIssuedAt *string `json:"migration_card_issued_at,omitempty"`
	MigrationCardExpiresAt *string `json:"migration_card_expires_at,omitempty"`
	// ResidenceDocType Type of legal-stay document: РВП / ВНЖ / виза
	ResidenceDocType *string `json:"residence_doc_type,omitempty"`
	ResidenceDocNumber *string `json:"residence_doc_number,omitempty"`
	ResidenceDocIssuedAt *string `json:"residence_doc_issued_at,omitempty"`
	ResidenceDocExpiresAt *string `json:"residence_doc_expires_at,omitempty"`
}

// PDLDeclaration Politically-exposed-person declaration (этап 4)
type PDLDeclaration struct {
	IsPdl bool `json:"is_pdl"`
	Category *PDLCategory `json:"category,omitempty"`
	// Position Position of the PEP — required when is_pdl=true
	Position *string `json:"position,omitempty"`
	Relation *PDLRelation `json:"relation,omitempty"`
}

// OwnershipChainLink Single hop in the ownership chain leading to a UBO (этап 5)
type OwnershipChainLink struct {
	// Level Depth of the link, 1 = direct parent
	Level int64 `json:"level"`
	EntityName string `json:"entity_name"`
	// EntityINNOrRegNumber INN for RU entities, foreign registration number otherwise
	EntityINNOrRegNumber *string `json:"entity_inn_or_reg_number,omitempty"`
	Country *string `json:"country,omitempty"`
	SharePercent float64 `json:"share_percent"`
}

// FATCADeclaration FATCA / CRS tax-residency declaration (этап 5)
type FATCADeclaration struct {
	TaxResidencyCountries []string `json:"tax_residency_countries,omitempty"`
	// TinPerCountry TIN per residency country
	TinPerCountry []map[string]interface{} `json:"tin_per_country,omitempty"`
	// UsPerson True if subject is US person under FATCA
	UsPerson bool `json:"us_person"`
	// FormDocID W-8 / W-9 form upload
	FormDocID *string `json:"form_doc_id,omitempty"`
}

// ScreeningResult Outcome of a single watch-list screen (этап 7)
type ScreeningResult struct {
	// ListName Identifier of the watch list, e.g. 'OFAC_SDN', 'EU_CFSP', 'UK_HMT', 'ROSFINMON_TERROR'
	ListName string `json:"list_name"`
	MatchLevel ScreeningMatchLevel `json:"match_level"`
	// Score Fuzzy-match score 0–1 (1 = exact match)
	Score *float64 `json:"score,omitempty"`
	MatchedEntityName *string `json:"matched_entity_name,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// AdverseMediaHit Negative-news article matching the subject (этап 7)
type AdverseMediaHit struct {
	Category AdverseMediaCategory `json:"category"`
	Title string `json:"title"`
	URL string `json:"url"`
	Source *string `json:"source,omitempty"`
	PublishedAt string `json:"published_at"`
	Summary *string `json:"summary,omitempty"`
}

// AntiFraudSignals Submission-time anti-fraud signals (этап 7)
type AntiFraudSignals struct {
	DeviceFingerprint *string `json:"device_fingerprint,omitempty"`
	IPAddress *string `json:"ip_address,omitempty"`
	Geolocation map[string]interface{} `json:"geolocation,omitempty"`
}

// MonitoringRule 375-П / per-tenant monitoring rule applied to the account (этап 10)
type MonitoringRule struct {
	// Code Rule identifier, e.g. '375-P-2.7' / 'CASH-LIMIT-DAILY'
	Code string `json:"code"`
	Description string `json:"description"`
	// Params Rule-specific parameters (thresholds, lists, etc.)
	Params map[string]interface{} `json:"params,omitempty"`
}

// TransactionLimits Per-operation-type limits applied at account opening (этап 8/9)
type TransactionLimits struct {
	DailyOutgoing *MoneyAmount `json:"daily_outgoing,omitempty"`
	DailyCashWithdrawal *MoneyAmount `json:"daily_cash_withdrawal,omitempty"`
	MonthlyOutgoing *MoneyAmount `json:"monthly_outgoing,omitempty"`
	SingleTransactionMax *MoneyAmount `json:"single_transaction_max,omitempty"`
}

// AccountAgreements Bundle of consents and agreements signed at account opening (этап 9)
type AccountAgreements struct {
	AgreementAcceptance bool `json:"agreement_acceptance"`
	AgreementAcceptedAt time.Time `json:"agreement_accepted_at"`
	DboAgreement *bool `json:"dbo_agreement,omitempty"`
	DboChannels []RemoteBankingChannel `json:"dbo_channels,omitempty"`
	EdoAgreement *bool `json:"edo_agreement,omitempty"`
	PersonalDataConsent bool `json:"personal_data_consent"`
	SigningMethod SigningMethod `json:"signing_method"`
	// UkepCertificateSerial Serial number of the UKEP certificate — required when signing_method=ukep
	UkepCertificateSerial *string `json:"ukep_certificate_serial,omitempty"`
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
	// PrequalificationCheckID Result of the pre-qualification stage (этап 1)
	PrequalificationCheckID *string `json:"prequalification_check_id,omitempty"`
	// LegalEntityProfileID Extended legal-entity questionnaire (этап 2)
	LegalEntityProfileID *string `json:"legal_entity_profile_id,omitempty"`
	// ActivityID AML activity declaration (этап 3)
	ActivityID *string `json:"activity_id,omitempty"`
	// ScreeningSetID Aggregated AML screening results (этап 7)
	ScreeningSetID *string `json:"screening_set_id,omitempty"`
	// MonitoringProfileID Continuous monitoring profile (этап 10)
	MonitoringProfileID *string `json:"monitoring_profile_id,omitempty"`
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
	// BirthPlace Place of birth as printed on the identity document (этап 4)
	BirthPlace *string `json:"birth_place,omitempty"`
	// Citizenship Citizenship(s) — array поддерживает двойное гражданство (этап 4)
	Citizenship []string `json:"citizenship,omitempty"`
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
	// IDDocument Generalised identity document (passport_ru / passport_foreign / refugee). Дублирует passport_* поля для не-RU граждан (этап 4). Add-only — старые passport_* остаются required.
	IDDocument *IDDocument `json:"id_document,omitempty"`
	// RegistrationAddress Place of permanent registration (прописка) — этап 4
	RegistrationAddress *StructuredAddress `json:"registration_address,omitempty"`
	// ActualAddress Place of actual residence — этап 4
	ActualAddress *StructuredAddress `json:"actual_address,omitempty"`
	// ForeignerInfo Migration card / residence permit — для иностранцев (этап 4)
	ForeignerInfo *ForeignerInfo `json:"foreigner_info,omitempty"`
	// PdlDeclaration Politically-exposed-person declaration (этап 4)
	PdlDeclaration *PDLDeclaration `json:"pdl_declaration,omitempty"`
	// FatcaDeclaration FATCA / CRS declaration — required для UBO (этап 5)
	FatcaDeclaration *FATCADeclaration `json:"fatca_declaration,omitempty"`
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
	// LegalAddress Registered legal address as plain text (legacy). Структурированный аналог — в LegalEntityProfile.
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
	// ProfileID Extended profile populated during onboarding (этап 2). Add-only optional reference.
	ProfileID *string `json:"profile_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LegalEntityProfile Расширенная анкета юрлица (этап 2). Дополняет LegalEntity полями, которые собираются при онбординге, но не входят в core-сущность ЕГРЮЛ. Связь — через LegalEntity.profile_id.
type LegalEntityProfile struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	LegalEntityID string `json:"legal_entity_id"`
	OpfCode *string `json:"opf_code,omitempty"`
	RegistrationAuthority *string `json:"registration_authority,omitempty"`
	AuthorizedCapital *MoneyAmount `json:"authorized_capital,omitempty"`
	LegalAddressStruct *StructuredAddress `json:"legal_address_struct,omitempty"`
	ActualAddressStruct *StructuredAddress `json:"actual_address_struct,omitempty"`
	ActualSameAsLegal *bool `json:"actual_same_as_legal,omitempty"`
	PostalAddressStruct *StructuredAddress `json:"postal_address_struct,omitempty"`
	PostalSameAsLegal *bool `json:"postal_same_as_legal,omitempty"`
	// OkvedMainV2 Validated OKVED2 main code (XX.XX.XX). Дублирует LegalEntity.okved_primary в structured формате.
	OkvedMainV2 *string `json:"okved_main_v2,omitempty"`
	OkvedAdditionalV2 []string `json:"okved_additional_v2,omitempty"`
	Licenses []LicenseInfo `json:"licenses,omitempty"`
	SroMembership []SROMembership `json:"sro_membership,omitempty"`
	Contacts *ContactInfo `json:"contacts,omitempty"`
	EmployeesCount *int64 `json:"employees_count,omitempty"`
	RevenueLastYear *MoneyAmount `json:"revenue_last_year,omitempty"`
	TaxRegime *TaxRegime `json:"tax_regime,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ApplicationActivity AML-сведения о деятельности заявителя (этап 3). Один-к-одному к Application.
type ApplicationActivity struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	BusinessDescription string `json:"business_description"`
	BusinessCategory BusinessRiskCategory `json:"business_category"`
	TopSuppliers []Counterparty `json:"top_suppliers,omitempty"`
	TopBuyers []Counterparty `json:"top_buyers,omitempty"`
	OperationalModel *OperationalModel `json:"operational_model,omitempty"`
	FundsSource FundsSource `json:"funds_source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Representative Единоличный исполнительный орган (ЕИО) или иной представитель юрлица (этап 4). Person хранит персональные данные, Representative — связь с заявкой и полномочия.
type Representative struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	LegalEntityID string `json:"legal_entity_id"`
	PersonID string `json:"person_id"`
	Authority AuthorityInfo `json:"authority"`
	// IsPrimary True для основного ЕИО (генерального директора)
	IsPrimary bool `json:"is_primary"`
	// IsSignatory True если лицо имеет право подписи финансовых документов
	IsSignatory *bool `json:"is_signatory,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PrequalificationCheck Результат предварительной проверки по ИНН/ОГРН/short_name (этап 1) — агрегирует ответы внешних источников и финальное решение proceed/manual/reject.
type PrequalificationCheck struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	INN string `json:"inn"`
	OGRN string `json:"ogrn"`
	// ShortNameHint Заявленное короткое наименование — сравнивается с ЕГРЮЛ для контроля соответствия
	ShortNameHint *string `json:"short_name_hint,omitempty"`
	EgrulStatus *LegalEntityStatus `json:"egrul_status,omitempty"`
	EgrulRegistrationDate *string `json:"egrul_registration_date,omitempty"`
	EgrulAddress *string `json:"egrul_address,omitempty"`
	EgrulCeoName *string `json:"egrul_ceo_name,omitempty"`
	EgrulFoundersSummary *string `json:"egrul_founders_summary,omitempty"`
	ZskLight *ZSKLight `json:"zsk_light,omitempty"`
	ZskAssignedAt *string `json:"zsk_assigned_at,omitempty"`
	// P639Present Запись в реестре отказников по 639-П
	P639Present *bool `json:"p639_present,omitempty"`
	P639Reason *string `json:"p639_reason,omitempty"`
	SanctionsResults []ScreeningResult `json:"sanctions_results,omitempty"`
	// RosfinmonPresent Запись в перечне Росфинмониторинга (террористы/экстремисты)
	RosfinmonPresent *bool `json:"rosfinmon_present,omitempty"`
	Decision PrequalificationDecision `json:"decision"`
	DecisionReason *string `json:"decision_reason,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ScreeningResultSet Набор сводных AML-проверок по заявке (этап 7). Включает санкции/PEP/adverse media по всем лицам + фрод-сигналы.
type ScreeningResultSet struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	SanctionsResults []ScreeningResult `json:"sanctions_results,omitempty"`
	PepResults []ScreeningResult `json:"pep_results,omitempty"`
	AdverseMediaHits []AdverseMediaHit `json:"adverse_media_hits,omitempty"`
	// OkvedConsistencyScore Соответствие ОКВЭД и заявленной деятельности (0–100)
	OkvedConsistencyScore *float64 `json:"okved_consistency_score,omitempty"`
	// TurnoverRealismScore Реалистичность планируемых оборотов vs ОКВЭД и численность
	TurnoverRealismScore *float64 `json:"turnover_realism_score,omitempty"`
	AntiFraudSignals *AntiFraudSignals `json:"anti_fraud_signals,omitempty"`
	PerformedAt time.Time `json:"performed_at"`
	CreatedAt time.Time `json:"created_at"`
}

// MonitoringProfile Параметры постоянного мониторинга открытого счёта (этап 10). Создаётся одновременно с Account.
type MonitoringProfile struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	ApplicationID string `json:"application_id"`
	AccountID string `json:"account_id"`
	ReviewFrequencyMonths int64 `json:"review_frequency_months"`
	NextReviewDate string `json:"next_review_date"`
	MonitoringRules []MonitoringRule `json:"monitoring_rules,omitempty"`
	KycRefreshTriggers []KYCRefreshTrigger `json:"kyc_refresh_triggers,omitempty"`
	TransactionLimits *TransactionLimits `json:"transaction_limits,omitempty"`
	NotificationChannels []string `json:"notification_channels,omitempty"`
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
	// OwnershipChains Линейные цепочки владения (этап 5). Add-only поле — расчёт может ленивым образом подтягивать из nodes/edges.
	OwnershipChains []map[string]interface{} `json:"ownership_chains,omitempty"`
	// NoUBOReason Если УБО не определены — обоснование (этап 5)
	NoUBOReason *string `json:"no_ubo_reason,omitempty"`
	// EioAsUBOConfirmation Подтверждение что ЕИО автоматически признан УБО (этап 5)
	EioAsUBOConfirmation *bool `json:"eio_as_ubo_confirmation,omitempty"`
	// DiagramDocID Загруженная схема цепочки владения (этап 5)
	DiagramDocID *string `json:"diagram_doc_id,omitempty"`
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
	IssueDate *string `json:"issue_date,omitempty"`
	ExpiryDate *string `json:"expiry_date,omitempty"`
	// SignedByPersonID Лицо, подписавшее документ (этап 6)
	SignedByPersonID *string `json:"signed_by_person_id,omitempty"`
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
	// MonitoringProfileID Связанный профиль мониторинга — заполняется на этапе 8 при выборе режима наблюдения
	MonitoringProfileID *string `json:"monitoring_profile_id,omitempty"`
	// TransactionLimits Лимиты, рекомендованные риск-движком (этап 8)
	TransactionLimits *TransactionLimits `json:"transaction_limits,omitempty"`
	// ScreeningSetID Набор AML-проверок, использованный риск-движком (этап 7)
	ScreeningSetID *string `json:"screening_set_id,omitempty"`
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
	// CorrespondentAccount Корреспондентский счёт банка (этап 9)
	CorrespondentAccount *string `json:"correspondent_account,omitempty"`
	TariffPlan *string `json:"tariff_plan,omitempty"`
	Agreements *AccountAgreements `json:"agreements,omitempty"`
	// MonitoringProfileID Профиль постоянного мониторинга, привязанный к счёту (этап 10)
	MonitoringProfileID *string `json:"monitoring_profile_id,omitempty"`
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

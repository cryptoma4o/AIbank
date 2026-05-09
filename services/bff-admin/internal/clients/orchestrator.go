package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Application — admin-payload заявки.
type Application struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	ApplicantID     string    `json:"applicant_id"`
	LegalEntityType string    `json:"legal_entity_type"`
	Channel         string    `json:"channel"`
	State           string    `json:"state"`
	ProductCodes    []string  `json:"product_codes"`
	WorkflowID      string    `json:"workflow_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ── Этапы 2-10 формы онбординга ──────────────────────────────────────
//
// Read-only payload-типы для admin-просмотра данных, которые applicant
// заполняет в bff-onboarding. Зеркалят domain-типы из
// services/onboarding-orchestrator/internal/domain/*.go (MUST stay in sync).
//
// Все типы могут быть nil (404 → ErrNotFound), если applicant ещё не
// дошёл до соответствующего этапа.

// LegalEntityProfile — этап 2 формы (расширенная анкета юрлица).
type LegalEntityProfile struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	ApplicationID         string             `json:"application_id"`
	LegalEntityID         string             `json:"legal_entity_id"`
	OPFCode               string             `json:"opf_code,omitempty"`
	RegistrationAuthority string             `json:"registration_authority,omitempty"`
	AuthorizedCapital     *MoneyAmount       `json:"authorized_capital,omitempty"`
	LegalAddress          *StructuredAddress `json:"legal_address_struct,omitempty"`
	ActualAddress         *StructuredAddress `json:"actual_address_struct,omitempty"`
	ActualSameAsLegal     bool               `json:"actual_same_as_legal"`
	PostalAddress         *StructuredAddress `json:"postal_address_struct,omitempty"`
	PostalSameAsLegal     bool               `json:"postal_same_as_legal"`
	OKVEDMain             string             `json:"okved_main_v2,omitempty"`
	OKVEDAdditional       []string           `json:"okved_additional_v2,omitempty"`
	Licenses              []LicenseInfo      `json:"licenses,omitempty"`
	SROMembership         []SROMembership    `json:"sro_membership,omitempty"`
	Contacts              *ContactInfo       `json:"contacts,omitempty"`
	EmployeesCount        *int               `json:"employees_count,omitempty"`
	RevenueLastYear       *MoneyAmount       `json:"revenue_last_year,omitempty"`
	TaxRegime             string             `json:"tax_regime,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// StructuredAddress — структурированный адрес.
type StructuredAddress struct {
	CountryCode string `json:"country_code"`
	PostalCode  string `json:"postal_code,omitempty"`
	RegionCode  string `json:"region_code,omitempty"`
	RegionName  string `json:"region_name,omitempty"`
	City        string `json:"city"`
	Street      string `json:"street,omitempty"`
	Building    string `json:"building,omitempty"`
	Office      string `json:"office,omitempty"`
	FIASID      string `json:"fias_id,omitempty"`
}

// MoneyAmount — сумма + валюта (ISO 4217).
type MoneyAmount struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// ContactInfo — контакты юрлица.
type ContactInfo struct {
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
	Website string `json:"website,omitempty"`
}

// LicenseInfo — лицензия / разрешение.
type LicenseInfo struct {
	Number       string `json:"number"`
	IssueDate    string `json:"issue_date"`
	ExpiryDate   string `json:"expiry_date,omitempty"`
	Issuer       string `json:"issuer"`
	ActivityType string `json:"activity_type"`
}

// SROMembership — членство в СРО.
type SROMembership struct {
	Name      string `json:"name"`
	RegNumber string `json:"reg_number"`
	JoinDate  string `json:"join_date"`
}

// ApplicationActivity — этап 3 (AML-анкета).
type ApplicationActivity struct {
	ID                  string            `json:"id"`
	TenantID            string            `json:"tenant_id"`
	ApplicationID       string            `json:"application_id"`
	BusinessDescription string            `json:"business_description"`
	BusinessCategory    string            `json:"business_category"`
	TopSuppliers        []Counterparty    `json:"top_suppliers,omitempty"`
	TopBuyers           []Counterparty    `json:"top_buyers,omitempty"`
	OperationalModel    *OperationalModel `json:"operational_model,omitempty"`
	FundsSource         FundsSource       `json:"funds_source"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// Counterparty — top-5 поставщик/покупатель.
type Counterparty struct {
	Name             string  `json:"name"`
	INN              string  `json:"inn,omitempty"`
	Country          string  `json:"country"`
	SharePercent     float64 `json:"share_percent"`
	RelationshipType string  `json:"relationship_type"`
}

// OperationalModel — планируемая модель операций.
type OperationalModel struct {
	Geography               []string     `json:"geography,omitempty"`
	MonthlyTurnoverPlanned  *MoneyAmount `json:"monthly_turnover_planned,omitempty"`
	AnnualTurnoverPlanned   *MoneyAmount `json:"annual_turnover_planned,omitempty"`
	CashSharePercent        *float64     `json:"cash_share_percent,omitempty"`
	ForeignEconomicActivity bool         `json:"foreign_economic_activity"`
	ForeignCountries        []string     `json:"foreign_countries,omitempty"`
	CurrencyOperations      []string     `json:"currency_operations,omitempty"`
}

// FundsSource — источник средств.
type FundsSource struct {
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
}

// Representative — этап 4 (ЕИО / представители).
type Representative struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	ApplicationID       string             `json:"application_id"`
	LegalEntityID       string             `json:"legal_entity_id"`
	LastName            string             `json:"last_name"`
	FirstName           string             `json:"first_name"`
	MiddleName          string             `json:"middle_name,omitempty"`
	BirthDate           string             `json:"birth_date"`
	BirthPlace          string             `json:"birth_place,omitempty"`
	Citizenship         []string           `json:"citizenship,omitempty"`
	INN                 string             `json:"inn,omitempty"`
	SNILS               string             `json:"snils,omitempty"`
	IDDocument          IDDocument         `json:"id_document"`
	RegistrationAddress *StructuredAddress `json:"registration_address,omitempty"`
	ActualAddress       *StructuredAddress `json:"actual_address,omitempty"`
	ForeignerInfo       *ForeignerInfo     `json:"foreigner_info,omitempty"`
	Authority           AuthorityInfo      `json:"authority"`
	PDLDeclaration      *PDLDeclaration    `json:"pdl_declaration,omitempty"`
	IsPrimary           bool               `json:"is_primary"`
	IsSignatory         bool               `json:"is_signatory"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

// IDDocument — документ, удостоверяющий личность.
type IDDocument struct {
	DocType        string `json:"doc_type"`
	Series         string `json:"series,omitempty"`
	Number         string `json:"number"`
	IssueDate      string `json:"issue_date,omitempty"`
	ExpiryDate     string `json:"expiry_date,omitempty"`
	IssuedBy       string `json:"issued_by,omitempty"`
	DepartmentCode string `json:"department_code,omitempty"`
}

// AuthorityInfo — полномочия представителя.
type AuthorityInfo struct {
	Position             string `json:"position"`
	AuthorityBasis       string `json:"authority_basis"`
	AuthorityDocNumber   string `json:"authority_doc_number,omitempty"`
	AuthorityDocDate     string `json:"authority_doc_date,omitempty"`
	SignatureSampleDocID string `json:"signature_sample_doc_id,omitempty"`
}

// ForeignerInfo — для иностранных граждан.
type ForeignerInfo struct {
	MigrationCardNumber    string `json:"migration_card_number,omitempty"`
	MigrationCardIssuedAt  string `json:"migration_card_issued_at,omitempty"`
	MigrationCardExpiresAt string `json:"migration_card_expires_at,omitempty"`
	ResidenceDocType       string `json:"residence_doc_type,omitempty"`
	ResidenceDocNumber     string `json:"residence_doc_number,omitempty"`
	ResidenceDocIssuedAt   string `json:"residence_doc_issued_at,omitempty"`
	ResidenceDocExpiresAt  string `json:"residence_doc_expires_at,omitempty"`
}

// PDLDeclaration — декларация ПДЛ.
type PDLDeclaration struct {
	IsPDL    bool   `json:"is_pdl"`
	Category string `json:"category,omitempty"`
	Position string `json:"position,omitempty"`
	Relation string `json:"relation,omitempty"`
}

// UBOGraph — этап 5 (граф владения).
type UBOGraph struct {
	ID                   string              `json:"id"`
	TenantID             string              `json:"tenant_id"`
	ApplicationID        string              `json:"application_id"`
	LegalEntityID        string              `json:"legal_entity_id"`
	Nodes                []UBONode           `json:"nodes"`
	Edges                []UBOEdge           `json:"edges"`
	OwnershipChains      []UBOOwnershipChain `json:"ownership_chains,omitempty"`
	NoUBOReason          string              `json:"no_ubo_reason,omitempty"`
	EIOAsUBOConfirmation bool                `json:"eio_as_ubo_confirmation"`
	DiagramDocID         string              `json:"diagram_doc_id,omitempty"`
	ComputedAt           *time.Time          `json:"computed_at,omitempty"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
}

// UBONode — узел графа владения.
type UBONode struct {
	ID               string            `json:"id"`
	NodeType         string            `json:"node_type"`
	Name             string            `json:"name"`
	PersonID         string            `json:"person_id,omitempty"`
	LegalEntityID    string            `json:"legal_entity_id,omitempty"`
	DirectStake      float64           `json:"direct_stake"`
	EffectiveStake   float64           `json:"effective_stake"`
	IsUBO            bool              `json:"is_ubo"`
	ControlBasis     string            `json:"control_basis,omitempty"`
	FATCADeclaration *FATCADeclaration `json:"fatca_declaration,omitempty"`
}

// UBOEdge — ребро владения.
type UBOEdge struct {
	FromNodeID string  `json:"from_node_id"`
	ToNodeID   string  `json:"to_node_id"`
	Stake      float64 `json:"stake"`
	DocumentID string  `json:"document_id,omitempty"`
}

// UBOOwnershipChain — цепочка владения от UBO к юрлицу.
type UBOOwnershipChain struct {
	UBONodeID         string               `json:"ubo_node_id"`
	Links             []OwnershipChainLink `json:"links"`
	IsSoleBeneficiary bool                 `json:"is_sole_beneficiary"`
}

// OwnershipChainLink — звено цепочки владения.
type OwnershipChainLink struct {
	Level                int     `json:"level"`
	EntityName           string  `json:"entity_name"`
	EntityINNOrRegNumber string  `json:"entity_inn_or_reg_number,omitempty"`
	Country              string  `json:"country,omitempty"`
	SharePercent         float64 `json:"share_percent"`
}

// FATCADeclaration — FATCA/CRS-декларация UBO.
type FATCADeclaration struct {
	TaxResidencyCountries []string   `json:"tax_residency_countries,omitempty"`
	TINPerCountry         []FATCATIN `json:"tin_per_country,omitempty"`
	USPerson              bool       `json:"us_person"`
	FormDocID             string     `json:"form_doc_id,omitempty"`
}

// FATCATIN — TIN по стране резидентства.
type FATCATIN struct {
	Country string `json:"country"`
	TIN     string `json:"tin"`
}

// ScreeningResultSet — этап 7 (скрининг).
type ScreeningResultSet struct {
	ID                    string            `json:"id"`
	TenantID              string            `json:"tenant_id"`
	ApplicationID         string            `json:"application_id"`
	SanctionsResults      []ScreeningResult `json:"sanctions_results,omitempty"`
	PEPResults            []ScreeningResult `json:"pep_results,omitempty"`
	AdverseMediaHits      []AdverseMediaHit `json:"adverse_media_hits,omitempty"`
	OKVEDConsistencyScore *float64          `json:"okved_consistency_score,omitempty"`
	TurnoverRealismScore  *float64          `json:"turnover_realism_score,omitempty"`
	AntiFraudSignals      *AntiFraudSignals `json:"anti_fraud_signals,omitempty"`
	PerformedAt           time.Time         `json:"performed_at"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// ScreeningResult — отметка matching по watch list.
type ScreeningResult struct {
	ListName          string     `json:"list_name"`
	MatchLevel        string     `json:"match_level"`
	Score             float64    `json:"score,omitempty"`
	MatchedEntityName string     `json:"matched_entity_name,omitempty"`
	CheckedAt         *time.Time `json:"checked_at,omitempty"`
}

// AdverseMediaHit — найденная негативная публикация.
type AdverseMediaHit struct {
	Category    string `json:"category"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Source      string `json:"source,omitempty"`
	PublishedAt string `json:"published_at"`
	Summary     string `json:"summary,omitempty"`
}

// AntiFraudSignals — фрод-сигналы при подаче заявки.
type AntiFraudSignals struct {
	DeviceFingerprint string       `json:"device_fingerprint,omitempty"`
	IPAddress         string       `json:"ip_address,omitempty"`
	Geolocation       *GeoLocation `json:"geolocation,omitempty"`
}

// GeoLocation — гео-метка по IP.
type GeoLocation struct {
	Country   string  `json:"country,omitempty"`
	City      string  `json:"city,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// MonitoringProfile — этапы 8/10 (мониторинг счёта).
type MonitoringProfile struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	ApplicationID         string             `json:"application_id"`
	AccountID             string             `json:"account_id,omitempty"`
	ReviewFrequencyMonths int                `json:"review_frequency_months"`
	NextReviewDate        string             `json:"next_review_date"`
	MonitoringRules       []MonitoringRule   `json:"monitoring_rules,omitempty"`
	KYCRefreshTriggers    []string           `json:"kyc_refresh_triggers,omitempty"`
	TransactionLimits     *TransactionLimits `json:"transaction_limits,omitempty"`
	NotificationChannels  []string           `json:"notification_channels,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// MonitoringRule — одно правило 375-П.
type MonitoringRule struct {
	Code        string         `json:"code"`
	Description string         `json:"description"`
	Params      map[string]any `json:"params,omitempty"`
}

// TransactionLimits — лимиты по типам операций.
type TransactionLimits struct {
	DailyOutgoing       *MoneyAmount `json:"daily_outgoing,omitempty"`
	DailyCashWithdrawal *MoneyAmount `json:"daily_cash_withdrawal,omitempty"`
	MonthlyOutgoing     *MoneyAmount `json:"monthly_outgoing,omitempty"`
	SingleTransactionMax *MoneyAmount `json:"single_transaction_max,omitempty"`
}

// BankAccount — этап 10 (открытый счёт). Назван BankAccount чтобы не
// конфликтовать с потенциальным Account-типом в GraphQL slice'е.
type BankAccount struct {
	ID                   string             `json:"id"`
	TenantID             string             `json:"tenant_id"`
	ApplicationID        string             `json:"application_id"`
	LegalEntityID        string             `json:"legal_entity_id"`
	AccountNumber        string             `json:"account_number,omitempty"`
	BIK                  string             `json:"bik,omitempty"`
	BankName             string             `json:"bank_name,omitempty"`
	Currency             string             `json:"currency"`
	AccountType          string             `json:"account_type"`
	CorrespondentAccount string             `json:"correspondent_account,omitempty"`
	TariffPlan           string             `json:"tariff_plan,omitempty"`
	Agreements           AccountAgreements  `json:"agreements"`
	MonitoringProfileID  string             `json:"monitoring_profile_id,omitempty"`
	ABSReference         string             `json:"abs_reference,omitempty"`
	OpenedAt             *time.Time         `json:"opened_at,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// AccountAgreements — пакет согласий и подписи при открытии счёта.
type AccountAgreements struct {
	AgreementAcceptance   bool      `json:"agreement_acceptance"`
	AgreementAcceptedAt   time.Time `json:"agreement_accepted_at"`
	DBOAgreement          bool      `json:"dbo_agreement"`
	DBOChannels           []string  `json:"dbo_channels,omitempty"`
	EDOAgreement          bool      `json:"edo_agreement"`
	PersonalDataConsent   bool      `json:"personal_data_consent"`
	SigningMethod         string    `json:"signing_method"`
	UKEPCertificateSerial string    `json:"ukep_certificate_serial,omitempty"`
}

// Decision — финальное решение по заявке.
type Decision struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Decision      string    `json:"decision"`
	Reasoning     string    `json:"reasoning"`
	DecidedAt     time.Time `json:"decided_at"`
}

// OrchestratorClient — клиент к onboarding-orchestrator.
type OrchestratorClient struct {
	baseURL string
	tr      *transport
}

func NewOrchestratorClient(baseURL string) *OrchestratorClient {
	return &OrchestratorClient{baseURL: baseURL, tr: newTransport()}
}

// ListFilter — параметры фильтрации.
type ListFilter struct {
	State           string
	LegalEntityType string
	Limit           int
}

// List — GET /v1/applications?tenant_id=...
func (c *OrchestratorClient) List(ctx context.Context, tenantID string, f ListFilter) ([]Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	if f.State != "" {
		q.Set("state", f.State)
	}
	if f.LegalEntityType != "" {
		q.Set("legal_entity_type", f.LegalEntityType)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	u := fmt.Sprintf("%s/v1/applications?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var resp struct {
		Items []Application `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// ListByDateRange — заявки тенанта с фильтром по created_at ∈ [from, to].
//
// Сегодня orchestrator не принимает date-фильтр на /v1/applications,
// поэтому фильтрация выполняется client-side.  TODO upstream: добавить
// query-параметры created_from/created_to, чтобы избежать выгрузки всего
// списка для тенантов с большим объёмом заявок.
func (c *OrchestratorClient) ListByDateRange(ctx context.Context, tenantID string, from, to time.Time) ([]Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	// Передаём фильтр на сервер (на случай если upstream уже умеет — экономим payload),
	// но всё равно делаем повторную фильтрацию на клиенте — она безопасна.
	if !from.IsZero() {
		q.Set("created_from", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("created_to", to.UTC().Format(time.RFC3339))
	}
	u := fmt.Sprintf("%s/v1/applications?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var resp struct {
		Items []Application `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	out := resp.Items[:0]
	for _, a := range resp.Items {
		if !from.IsZero() && a.CreatedAt.Before(from) {
			continue
		}
		if !to.IsZero() && a.CreatedAt.After(to) {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// Get — GET /v1/applications/{id}?tenant_id=...
func (c *OrchestratorClient) Get(ctx context.Context, tenantID, id string) (*Application, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/applications/%s?%s", c.baseURL, id, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var p Application
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateDecision — POST /v1/applications/{id}/decision.
//
// TransitionState — POST /v1/applications/{id}/transitions.
//
// Ручной перевод заявки между состояниями state machine. Используется,
// пока Temporal-activities не реализованы и workflow не двигает заявку
// автоматически — bank-оператор переключает её в админ-панели.
func (c *OrchestratorClient) TransitionState(ctx context.Context, tenantID, applicationID, newState, reason string) (*Application, error) {
	u := fmt.Sprintf("%s/v1/applications/%s/transitions", c.baseURL, applicationID)
	body := map[string]any{
		"tenant_id": tenantID,
		"new_state": newState,
		"reason":    reason,
	}
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	var app Application
	if err := c.tr.doJSON(req, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// Используется для ручных решений в manual_review состоянии.
func (c *OrchestratorClient) UpdateDecision(ctx context.Context, tenantID, applicationID, decision, reasoning string) (*Decision, error) {
	u := fmt.Sprintf("%s/v1/applications/%s/decision", c.baseURL, applicationID)
	body := map[string]any{
		"tenant_id": tenantID,
		"decision":  decision,
		"reasoning": reasoning,
	}
	req, err := newJSONRequest(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, err
	}
	var d Decision
	if err := c.tr.doJSON(req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ── Read-only GET по этапам 2-10 формы онбординга ────────────────────
//
// Все методы используют один паттерн: GET {base}/v1/{resource}/by-application/{id}?tenant_id=...
// При 404 транспорт возвращает ErrNotFound — caller (resolver) интерпретирует
// его как nil без ошибки (applicant ещё не дошёл до этапа).

// GetLegalEntityProfile — этап 2.
func (c *OrchestratorClient) GetLegalEntityProfile(ctx context.Context, tenantID, applicationID string) (*LegalEntityProfile, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/legal-entity-profiles/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var p LegalEntityProfile
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetApplicationActivity — этап 3 (AML-анкета).
func (c *OrchestratorClient) GetApplicationActivity(ctx context.Context, tenantID, applicationID string) (*ApplicationActivity, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/application-activities/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var a ApplicationActivity
	if err := c.tr.doJSON(req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ListRepresentatives — этап 4. Orchestrator возвращает {items: [...]}.
func (c *OrchestratorClient) ListRepresentatives(ctx context.Context, tenantID, applicationID string) ([]Representative, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/representatives/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []Representative `json:"items"`
	}
	if err := c.tr.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// GetUBOGraph — этап 5.
func (c *OrchestratorClient) GetUBOGraph(ctx context.Context, tenantID, applicationID string) (*UBOGraph, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/ubo-graphs/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var g UBOGraph
	if err := c.tr.doJSON(req, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// GetScreening — этап 7.
func (c *OrchestratorClient) GetScreening(ctx context.Context, tenantID, applicationID string) (*ScreeningResultSet, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/screenings/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var s ScreeningResultSet
	if err := c.tr.doJSON(req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetMonitoringProfile — этапы 8/10.
func (c *OrchestratorClient) GetMonitoringProfile(ctx context.Context, tenantID, applicationID string) (*MonitoringProfile, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/monitoring-profiles/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var p MonitoringProfile
	if err := c.tr.doJSON(req, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetAccount — этап 10 (открытый счёт).
func (c *OrchestratorClient) GetAccount(ctx context.Context, tenantID, applicationID string) (*BankAccount, error) {
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	u := fmt.Sprintf("%s/v1/accounts/by-application/%s?%s", c.baseURL, applicationID, q.Encode())
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var a BankAccount
	if err := c.tr.doJSON(req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

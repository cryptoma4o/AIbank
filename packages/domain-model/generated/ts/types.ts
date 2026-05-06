// Code generated from schema.json. DO NOT EDIT.
// Source: schema.json (https://aibank.io/schemas/domain-model/v1.1.0/schema.json)

/** State machine status for an onboarding application */
export type ApplicationStatus = "draft" | "documents_pending" | "validation_in_progress" | "risk_scoring" | "manual_review" | "auto_approved" | "rejected" | "account_opening" | "completed" | "cancelled";

export const ApplicationStatusValues = [
  "draft",
  "documents_pending",
  "validation_in_progress",
  "risk_scoring",
  "manual_review",
  "auto_approved",
  "rejected",
  "account_opening",
  "completed",
  "cancelled",
] as const;

/** Semantic category of an uploaded document. Расширено в v1.1.0 для покрытия этапа 6 формы (см. docs/onboarding-form-spec.md). Add-only: старые значения сохранены. */
export type DocumentType = "passport" | "inn_certificate" | "ogrn_certificate" | "charter" | "beneficial_owners_registry" | "ceo_appointment_order" | "bank_account_application" | "power_of_attorney" | "financial_statements" | "other" | "egrul_record_sheet" | "eio_appointment_protocol" | "eio_order" | "signature_card" | "license" | "address_confirmation" | "tax_declaration" | "tax_clearance_certificate" | "ownership_chain_diagram" | "fatca_w8_form" | "fatca_w9_form" | "migration_card" | "residence_permit" | "foreign_passport";

export const DocumentTypeValues = [
  "passport",
  "inn_certificate",
  "ogrn_certificate",
  "charter",
  "beneficial_owners_registry",
  "ceo_appointment_order",
  "bank_account_application",
  "power_of_attorney",
  "financial_statements",
  "other",
  "egrul_record_sheet",
  "eio_appointment_protocol",
  "eio_order",
  "signature_card",
  "license",
  "address_confirmation",
  "tax_declaration",
  "tax_clearance_certificate",
  "ownership_chain_diagram",
  "fatca_w8_form",
  "fatca_w9_form",
  "migration_card",
  "residence_permit",
  "foreign_passport",
] as const;

/** Processing state of a document */
export type DocumentStatus = "uploaded" | "processing" | "verified" | "rejected";

export const DocumentStatusValues = [
  "uploaded",
  "processing",
  "verified",
  "rejected",
] as const;

/** Legal organisational form of a Russian entity */
export type LegalForm = "ooo" | "ao" | "pao" | "ip" | "zao";

export const LegalFormValues = [
  "ooo",
  "ao",
  "pao",
  "ip",
  "zao",
] as const;

/** Lifecycle status of a legal entity from the state register */
export type LegalEntityStatus = "active" | "liquidating" | "liquidated" | "reorganizing";

export const LegalEntityStatusValues = [
  "active",
  "liquidating",
  "liquidated",
  "reorganizing",
] as const;

/** Bucketed risk classification */
export type RiskLevel = "low" | "medium" | "high" | "critical";

export const RiskLevelValues = [
  "low",
  "medium",
  "high",
  "critical",
] as const;

/** Model recommendation based on risk score */
export type Recommendation = "approve" | "manual_review" | "reject";

export const RecommendationValues = [
  "approve",
  "manual_review",
  "reject",
] as const;

/** How and in which direction the decision was made */
export type DecisionType = "auto_approved" | "auto_rejected" | "manual_approved" | "manual_rejected" | "escalated";

export const DecisionTypeValues = [
  "auto_approved",
  "auto_rejected",
  "manual_approved",
  "manual_rejected",
  "escalated",
] as const;

/** ISO 4217 currency code */
export type Currency = "RUB" | "USD" | "EUR";

export const CurrencyValues = [
  "RUB",
  "USD",
  "EUR",
] as const;

/** Functional type of a bank account. Add-only: расширено в v1.1.0 для покрытия этапа 9 (settlement/special/foreign_currency/escrow/nominal — см. docs/onboarding-form-spec.md §9). */
export type AccountType = "settlement" | "deposit" | "loan" | "special" | "foreign_currency" | "escrow" | "nominal";

export const AccountTypeValues = [
  "settlement",
  "deposit",
  "loan",
  "special",
  "foreign_currency",
  "escrow",
  "nominal",
] as const;

/** Type of electronic signature on a document */
export type SignatureType = "ukep" | "pep" | "none";

export const SignatureTypeValues = [
  "ukep",
  "pep",
  "none",
] as const;

/** Type of node in the beneficial ownership graph */
export type UBONodeType = "person" | "legal_entity";

export const UBONodeTypeValues = [
  "person",
  "legal_entity",
] as const;

/** Tax regime applicable to a Russian legal entity / IP */
export type TaxRegime = "osn" | "usn_income" | "usn_income_minus_expenses" | "psn" | "eshn" | "npd";

export const TaxRegimeValues = [
  "osn",
  "usn_income",
  "usn_income_minus_expenses",
  "psn",
  "eshn",
  "npd",
] as const;

/** Legal basis on which the representative acts on behalf of the legal entity (этап 4) */
export type AuthorityBasis = "charter" | "protocol" | "power_of_attorney" | "order";

export const AuthorityBasisValues = [
  "charter",
  "protocol",
  "power_of_attorney",
  "order",
] as const;

/** Type of identity document presented by a representative / UBO */
export type IDDocumentType = "passport_ru" | "passport_foreign" | "national_passport" | "refugee_certificate";

export const IDDocumentTypeValues = [
  "passport_ru",
  "passport_foreign",
  "national_passport",
  "refugee_certificate",
] as const;

/** Politically-exposed-person category — uses PEP terminology in regulatory texts */
export type PDLCategory = "foreign" | "russian" | "international_organization";

export const PDLCategoryValues = [
  "foreign",
  "russian",
  "international_organization",
] as const;

/** How the person relates to a politically-exposed-person status */
export type PDLRelation = "self" | "relative" | "representative";

export const PDLRelationValues = [
  "self",
  "relative",
  "representative",
] as const;

/** AML risk category of the declared OKVED activity (этап 3) */
export type BusinessRiskCategory = "low_risk" | "medium_risk" | "high_risk";

export const BusinessRiskCategoryValues = [
  "low_risk",
  "medium_risk",
  "high_risk",
] as const;

/** Declared origin of operating funds */
export type FundsSourceCategory = "revenue" | "founder_contribution" | "loan" | "investments" | "other";

export const FundsSourceCategoryValues = [
  "revenue",
  "founder_contribution",
  "loan",
  "investments",
  "other",
] as const;

/** Basis on which a person controls the legal entity (UBO — этап 5) */
export type ControlBasis = "capital_share" | "contract" | "other_decision_right";

export const ControlBasisValues = [
  "capital_share",
  "contract",
  "other_decision_right",
] as const;

/** Screening match outcome against a watch list */
export type ScreeningMatchLevel = "match" | "partial_match" | "no_match";

export const ScreeningMatchLevelValues = [
  "match",
  "partial_match",
  "no_match",
] as const;

/** Category of adverse media hit */
export type AdverseMediaCategory = "fraud" | "money_laundering" | "terrorism_financing" | "sanctions" | "corruption" | "other";

export const AdverseMediaCategoryValues = [
  "fraud",
  "money_laundering",
  "terrorism_financing",
  "sanctions",
  "corruption",
  "other",
] as const;

/** Counterparty relationship — постоянный / разовый */
export type RelationshipType = "regular" | "one_off";

export const RelationshipTypeValues = [
  "regular",
  "one_off",
] as const;

/** Event that triggers re-execution of KYC procedures */
export type KYCRefreshTrigger = "scheduled" | "ceo_change" | "ubo_change" | "ownership_change" | "address_change" | "license_expiry" | "transaction_threshold_exceeded" | "regulator_alert";

export const KYCRefreshTriggerValues = [
  "scheduled",
  "ceo_change",
  "ubo_change",
  "ownership_change",
  "address_change",
  "license_expiry",
  "transaction_threshold_exceeded",
  "regulator_alert",
] as const;

/** Method used to sign account-opening agreements */
export type SigningMethod = "ukep" | "sms_code" | "handwritten";

export const SigningMethodValues = [
  "ukep",
  "sms_code",
  "handwritten",
] as const;

/** Channel through which DBO (remote banking) is accessed */
export type RemoteBankingChannel = "web" | "mobile" | "api";

export const RemoteBankingChannelValues = [
  "web",
  "mobile",
  "api",
] as const;

/** ЗСК (Знай своего клиента) traffic-light category from CBR */
export type ZSKLight = "green" | "yellow" | "red" | "unknown";

export const ZSKLightValues = [
  "green",
  "yellow",
  "red",
  "unknown",
] as const;

/** Aggregated decision after the pre-qualification stage (этап 1) */
export type PrequalificationDecision = "proceed" | "manual_review" | "reject";

export const PrequalificationDecisionValues = [
  "proceed",
  "manual_review",
  "reject",
] as const;

/** A single factor contributing to a risk score */
export interface RiskFactor {
  /** Machine-readable factor identifier */
  name: string;
  /** Observed value of the factor (any scalar) */
  value: unknown;
  /** Signed integer contribution to the overall score (positive = higher risk) */
  impact: number;
  /** Human-readable explanation of how this factor affects the score */
  explanation: string;
}

/** A node (person or legal entity) in the beneficial ownership graph */
export interface UBONode {
  /** Node identifier within the graph */
  id: string;
  /** Reference to Person entity — present when node_type is person */
  person_id?: string;
  /** Reference to LegalEntity — present when node_type is legal_entity */
  legal_entity_id?: string;
  /** Display name of the node */
  name: string;
  node_type: UBONodeType;
  /** Direct ownership stake as a percentage (0–100) */
  direct_stake: number;
  /** Computed effective (indirect + direct) ownership stake as a percentage */
  effective_stake: number;
  /** True when effective_stake >= 25% (ultimate beneficial owner threshold) */
  is_ubo: boolean;
}

/** A directed ownership edge between two nodes in the beneficial ownership graph */
export interface UBOEdge {
  /** Source node identifier */
  from_node_id: string;
  /** Target node identifier */
  to_node_id: string;
  /** Ownership stake on this edge as a percentage */
  stake: number;
  /** Supporting document that evidences this ownership stake */
  document_id?: string;
}

/** Structured address per ФИАС, used in legal/actual/postal address blocks (этап 2) and для физлица (этап 4). */
export interface StructuredAddress {
  country_code: string;
  /** Postal index (free-form to support non-RU formats) */
  postal_code?: string;
  /** ФИАС region code — applicable for RU */
  region_code?: string;
  /** Region / state / province name */
  region_name?: string;
  /** City / locality */
  city: string;
  /** Street name */
  street?: string;
  /** Building / house number */
  building?: string;
  /** Apartment / office / floor */
  office?: string;
  /** ФИАС GUID, if resolved */
  fias_id?: string;
}

/** Monetary amount in a specific currency */
export interface MoneyAmount {
  /** Decimal amount (precision should match currency convention) */
  amount: number;
  currency: string;
}

/** Contact details for a legal entity (этап 2) */
export interface ContactInfo {
  phone?: string;
  email?: string;
  website?: string;
}

/** Licence held by a legal entity (этап 2) */
export interface LicenseInfo {
  number: string;
  issue_date: string;
  expiry_date?: string;
  issuer: string;
  activity_type: string;
}

/** Self-regulating organisation membership (этап 2) */
export interface SROMembership {
  name: string;
  reg_number: string;
  join_date: string;
}

/** Top supplier or top buyer counterparty (этап 3) */
export interface Counterparty {
  name: string;
  inn?: string;
  country: string;
  share_percent: number;
  relationship_type: RelationshipType;
}

/** Planned operational model (этап 3) */
export interface OperationalModel {
  geography?: string[];
  monthly_turnover_planned?: MoneyAmount;
  annual_turnover_planned?: MoneyAmount;
  cash_share_percent?: number;
  foreign_economic_activity?: boolean;
  foreign_countries?: string[];
  currency_operations?: string[];
}

/** Declared source of funds (этап 3) */
export interface FundsSource {
  category: FundsSourceCategory;
  /** Free-form justification — required when category=other */
  description?: string;
}

/** Identity document for a representative or UBO (этап 4/5) */
export interface IDDocument {
  doc_type: IDDocumentType;
  /** RU passport series (4 digits) — null for non-RU */
  series?: string;
  /** Document number (formats vary) */
  number: string;
  issue_date?: string;
  expiry_date?: string;
  issued_by?: string;
  /** RU department code (XXX-XXX) — null for non-RU */
  department_code?: string;
}

/** Powers of a representative (этап 4) */
export interface AuthorityInfo {
  position: string;
  authority_basis: AuthorityBasis;
  authority_doc_number?: string;
  authority_doc_date?: string;
  signature_sample_doc_id?: string;
}

/** Additional details required for non-RU citizens (этап 4) */
export interface ForeignerInfo {
  migration_card_number?: string;
  migration_card_issued_at?: string;
  migration_card_expires_at?: string;
  /** Type of legal-stay document: РВП / ВНЖ / виза */
  residence_doc_type?: string;
  residence_doc_number?: string;
  residence_doc_issued_at?: string;
  residence_doc_expires_at?: string;
}

/** Politically-exposed-person declaration (этап 4) */
export interface PDLDeclaration {
  is_pdl: boolean;
  category?: PDLCategory;
  /** Position of the PEP — required when is_pdl=true */
  position?: string;
  relation?: PDLRelation;
}

/** Single hop in the ownership chain leading to a UBO (этап 5) */
export interface OwnershipChainLink {
  /** Depth of the link, 1 = direct parent */
  level: number;
  entity_name: string;
  /** INN for RU entities, foreign registration number otherwise */
  entity_inn_or_reg_number?: string;
  country?: string;
  share_percent: number;
}

/** FATCA / CRS tax-residency declaration (этап 5) */
export interface FATCADeclaration {
  tax_residency_countries?: string[];
  /** TIN per residency country */
  tin_per_country?: Record<string, unknown>[];
  /** True if subject is US person under FATCA */
  us_person: boolean;
  /** W-8 / W-9 form upload */
  form_doc_id?: string;
}

/** Outcome of a single watch-list screen (этап 7) */
export interface ScreeningResult {
  /** Identifier of the watch list, e.g. 'OFAC_SDN', 'EU_CFSP', 'UK_HMT', 'ROSFINMON_TERROR' */
  list_name: string;
  match_level: ScreeningMatchLevel;
  /** Fuzzy-match score 0–1 (1 = exact match) */
  score?: number;
  matched_entity_name?: string;
  checked_at?: string;
}

/** Negative-news article matching the subject (этап 7) */
export interface AdverseMediaHit {
  category: AdverseMediaCategory;
  title: string;
  url: string;
  source?: string;
  published_at: string;
  summary?: string;
}

/** Submission-time anti-fraud signals (этап 7) */
export interface AntiFraudSignals {
  device_fingerprint?: string;
  ip_address?: string;
  geolocation?: Record<string, unknown>;
}

/** 375-П / per-tenant monitoring rule applied to the account (этап 10) */
export interface MonitoringRule {
  /** Rule identifier, e.g. '375-P-2.7' / 'CASH-LIMIT-DAILY' */
  code: string;
  description: string;
  /** Rule-specific parameters (thresholds, lists, etc.) */
  params?: Record<string, unknown>;
}

/** Per-operation-type limits applied at account opening (этап 8/9) */
export interface TransactionLimits {
  daily_outgoing?: MoneyAmount;
  daily_cash_withdrawal?: MoneyAmount;
  monthly_outgoing?: MoneyAmount;
  single_transaction_max?: MoneyAmount;
}

/** Bundle of consents and agreements signed at account opening (этап 9) */
export interface AccountAgreements {
  agreement_acceptance: boolean;
  agreement_accepted_at: string;
  dbo_agreement?: boolean;
  dbo_channels?: RemoteBankingChannel[];
  edo_agreement?: boolean;
  personal_data_consent: boolean;
  signing_method: SigningMethod;
  /** Serial number of the UKEP certificate — required when signing_method=ukep */
  ukep_certificate_serial?: string;
}

/** A bank that uses the AIbank platform. Root multi-tenant entity — every other entity belongs to exactly one Tenant. */
export interface Tenant {
  id: string;
  /** Display name, e.g. 'Альфа-Банк' */
  name: string;
  /** Kebab-case unique identifier used in URLs and config paths */
  slug: string;
  /** Lifecycle status of the tenant account */
  status: string;
  /** Semver of the tenant configuration currently applied */
  config_version: string;
  created_at: string;
  updated_at: string;
}

/** A single onboarding application. Core state-machine entity — one per legal entity from draft to account opening or rejection. */
export interface Application {
  id: string;
  tenant_id: string;
  /** External client reference (nullable) */
  client_id?: string;
  /** Legal entity under review (nullable until entity is created) */
  legal_entity_id?: string;
  status: ApplicationStatus;
  /** Temporal workflow execution ID tracking this application */
  workflow_id?: string;
  created_at: string;
  updated_at: string;
  /** When the applicant submitted the application (left draft state) */
  submitted_at?: string;
  /** When a final approve/reject decision was recorded */
  decided_at?: string;
  /** Opened account — null until status is completed */
  account_id?: string;
  /** Result of the pre-qualification stage (этап 1) */
  prequalification_check_id?: string;
  /** Extended legal-entity questionnaire (этап 2) */
  legal_entity_profile_id?: string;
  /** AML activity declaration (этап 3) */
  activity_id?: string;
  /** Aggregated AML screening results (этап 7) */
  screening_set_id?: string;
  /** Continuous monitoring profile (этап 10) */
  monitoring_profile_id?: string;
}

/** Any natural person in the system: applicant, director, founder, UBO, signatory. */
export interface Person {
  id: string;
  tenant_id: string;
  last_name: string;
  first_name: string;
  /** Patronymic (отчество), optional */
  middle_name?: string;
  /** Date of birth, UTC ISO 8601 */
  birth_date: string;
  /** Place of birth as printed on the identity document (этап 4) */
  birth_place?: string;
  /** Citizenship(s) — array поддерживает двойное гражданство (этап 4) */
  citizenship?: string[];
  inn: string;
  /** Insurance number — optional */
  snils?: string;
  /** Russian passport series (4 digits) */
  passport_series: string;
  /** Russian passport number (6 digits) */
  passport_number: string;
  /** Name of the issuing authority */
  passport_issued_by: string;
  /** Date the passport was issued, UTC ISO 8601 */
  passport_issued_at: string;
  /** Generalised identity document (passport_ru / passport_foreign / refugee). Дублирует passport_* поля для не-RU граждан (этап 4). Add-only — старые passport_* остаются required. */
  id_document?: IDDocument;
  /** Place of permanent registration (прописка) — этап 4 */
  registration_address?: StructuredAddress;
  /** Place of actual residence — этап 4 */
  actual_address?: StructuredAddress;
  /** Migration card / residence permit — для иностранцев (этап 4) */
  foreigner_info?: ForeignerInfo;
  /** Politically-exposed-person declaration (этап 4) */
  pdl_declaration?: PDLDeclaration;
  /** FATCA / CRS declaration — required для UBO (этап 5) */
  fatca_declaration?: FATCADeclaration;
  created_at: string;
}

/** A Russian legal entity (ООО, АО, etc.) undergoing or having completed onboarding. */
export interface LegalEntity {
  id: string;
  tenant_id: string;
  /** Full official name from EGRUL */
  full_name: string;
  /** Abbreviated name, e.g. 'ООО «Ромашка»' */
  short_name: string;
  inn: string;
  ogrn: string;
  /** KPP — nullable for individual entrepreneurs (IP) */
  kpp?: string;
  legal_form: LegalForm;
  /** Registered legal address as plain text (legacy). Структурированный аналог — в LegalEntityProfile. */
  legal_address: string;
  /** Actual operating address — nullable if same as legal_address */
  actual_address?: string;
  /** Primary OKVED activity code */
  okved_primary: string;
  /** Additional OKVED activity codes */
  okved_secondary: string[];
  /** Date of state registration */
  registration_date: string;
  status: LegalEntityStatus;
  /** Reference to the Person who is the current CEO/director */
  ceo_person_id: string;
  /** Extended profile populated during onboarding (этап 2). Add-only optional reference. */
  profile_id?: string;
  created_at: string;
  updated_at: string;
}

/** Расширенная анкета юрлица (этап 2). Дополняет LegalEntity полями, которые собираются при онбординге, но не входят в core-сущность ЕГРЮЛ. Связь — через LegalEntity.profile_id. */
export interface LegalEntityProfile {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  opf_code?: string;
  registration_authority?: string;
  authorized_capital?: MoneyAmount;
  legal_address_struct?: StructuredAddress;
  actual_address_struct?: StructuredAddress;
  actual_same_as_legal?: boolean;
  postal_address_struct?: StructuredAddress;
  postal_same_as_legal?: boolean;
  /** Validated OKVED2 main code (XX.XX.XX). Дублирует LegalEntity.okved_primary в structured формате. */
  okved_main_v2?: string;
  okved_additional_v2?: string[];
  licenses?: LicenseInfo[];
  sro_membership?: SROMembership[];
  contacts?: ContactInfo;
  employees_count?: number;
  revenue_last_year?: MoneyAmount;
  tax_regime?: TaxRegime;
  created_at: string;
  updated_at: string;
}

/** AML-сведения о деятельности заявителя (этап 3). Один-к-одному к Application. */
export interface ApplicationActivity {
  id: string;
  tenant_id: string;
  application_id: string;
  business_description: string;
  business_category: BusinessRiskCategory;
  top_suppliers?: Counterparty[];
  top_buyers?: Counterparty[];
  operational_model?: OperationalModel;
  funds_source: FundsSource;
  created_at: string;
  updated_at: string;
}

/** Единоличный исполнительный орган (ЕИО) или иной представитель юрлица (этап 4). Person хранит персональные данные, Representative — связь с заявкой и полномочия. */
export interface Representative {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  person_id: string;
  authority: AuthorityInfo;
  /** True для основного ЕИО (генерального директора) */
  is_primary: boolean;
  /** True если лицо имеет право подписи финансовых документов */
  is_signatory?: boolean;
  created_at: string;
  updated_at: string;
}

/** Результат предварительной проверки по ИНН/ОГРН/short_name (этап 1) — агрегирует ответы внешних источников и финальное решение proceed/manual/reject. */
export interface PrequalificationCheck {
  id: string;
  tenant_id: string;
  application_id: string;
  inn: string;
  ogrn: string;
  /** Заявленное короткое наименование — сравнивается с ЕГРЮЛ для контроля соответствия */
  short_name_hint?: string;
  egrul_status?: LegalEntityStatus;
  egrul_registration_date?: string;
  egrul_address?: string;
  egrul_ceo_name?: string;
  egrul_founders_summary?: string;
  zsk_light?: ZSKLight;
  zsk_assigned_at?: string;
  /** Запись в реестре отказников по 639-П */
  p639_present?: boolean;
  p639_reason?: string;
  sanctions_results?: ScreeningResult[];
  /** Запись в перечне Росфинмониторинга (террористы/экстремисты) */
  rosfinmon_present?: boolean;
  decision: PrequalificationDecision;
  decision_reason?: string;
  checked_at: string;
  created_at: string;
}

/** Набор сводных AML-проверок по заявке (этап 7). Включает санкции/PEP/adverse media по всем лицам + фрод-сигналы. */
export interface ScreeningResultSet {
  id: string;
  tenant_id: string;
  application_id: string;
  sanctions_results?: ScreeningResult[];
  pep_results?: ScreeningResult[];
  adverse_media_hits?: AdverseMediaHit[];
  /** Соответствие ОКВЭД и заявленной деятельности (0–100) */
  okved_consistency_score?: number;
  /** Реалистичность планируемых оборотов vs ОКВЭД и численность */
  turnover_realism_score?: number;
  anti_fraud_signals?: AntiFraudSignals;
  performed_at: string;
  created_at: string;
}

/** Параметры постоянного мониторинга открытого счёта (этап 10). Создаётся одновременно с Account. */
export interface MonitoringProfile {
  id: string;
  tenant_id: string;
  application_id: string;
  account_id: string;
  review_frequency_months: number;
  next_review_date: string;
  monitoring_rules?: MonitoringRule[];
  kyc_refresh_triggers?: KYCRefreshTrigger[];
  transaction_limits?: TransactionLimits;
  notification_channels?: string[];
  created_at: string;
  updated_at: string;
}

/** Computed beneficial ownership graph for one legal entity in the context of an application. */
export interface UBOGraph {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  /** All persons and entities in the ownership chain */
  nodes: UBONode[];
  /** Directed ownership edges between nodes */
  edges: UBOEdge[];
  /** Линейные цепочки владения (этап 5). Add-only поле — расчёт может ленивым образом подтягивать из nodes/edges. */
  ownership_chains?: Record<string, unknown>[];
  /** Если УБО не определены — обоснование (этап 5) */
  no_ubo_reason?: string;
  /** Подтверждение что ЕИО автоматически признан УБО (этап 5) */
  eio_as_ubo_confirmation?: boolean;
  /** Загруженная схема цепочки владения (этап 5) */
  diagram_doc_id?: string;
  computed_at: string;
}

/** A document uploaded by the applicant or retrieved from an external source. */
export interface Document {
  id: string;
  tenant_id: string;
  application_id: string;
  document_type: DocumentType;
  /** Original filename as provided by the uploader */
  filename: string;
  /** MIME type, e.g. 'application/pdf', 'image/jpeg' */
  mime_type: string;
  /** File size in bytes */
  size_bytes: number;
  /** Object storage key, e.g. 'tnt_xxx/app_yyy/doc_zzz/passport.pdf' */
  s3_key: string;
  /** Hex-encoded SHA-256 checksum of the file content */
  checksum_sha256: string;
  status: DocumentStatus;
  /** Key-value pairs extracted by OCR/ML — schema varies by document_type */
  extracted_fields?: Record<string, unknown>;
  /** Overall OCR confidence score (0–1); null when not yet processed */
  ocr_confidence?: number;
  /** When the document was electronically signed — null if unsigned */
  signed_at?: string;
  signature_type: SignatureType;
  issue_date?: string;
  expiry_date?: string;
  /** Лицо, подписавшее документ (этап 6) */
  signed_by_person_id?: string;
  created_at: string;
  updated_at: string;
}

/** ML-based risk scoring result for an onboarding application. */
export interface RiskAssessment {
  id: string;
  tenant_id: string;
  application_id: string;
  /** Risk score from 0 (lowest risk) to 100 (highest risk) */
  score: number;
  risk_level: RiskLevel;
  recommendation: Recommendation;
  /** Ordered list of factors that contributed to the score */
  factors: RiskFactor[];
  /** Semver of the risk model that produced this assessment */
  model_version: string;
  /** Связанный профиль мониторинга — заполняется на этапе 8 при выборе режима наблюдения */
  monitoring_profile_id?: string;
  /** Лимиты, рекомендованные риск-движком (этап 8) */
  transaction_limits?: TransactionLimits;
  /** Набор AML-проверок, использованный риск-движком (этап 7) */
  screening_set_id?: string;
  assessed_at: string;
}

/** The final underwriting decision on an onboarding application. */
export interface Decision {
  id: string;
  tenant_id: string;
  application_id: string;
  decision_type: DecisionType;
  /** User ID of the operator for manual decisions; null for automated decisions */
  actor_id?: string;
  /** Human-readable explanation — mandatory for rejections */
  reason: string;
  /** The risk assessment that informed this decision */
  risk_assessment_id: string;
  created_at: string;
}

/** A bank account opened for the legal entity after an approved application. */
export interface Account {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  account_number: string;
  bik: string;
  /** Name of the bank holding the account */
  bank_name: string;
  currency: Currency;
  account_type: AccountType;
  opened_at: string;
  /** Internal reference identifier in the bank's core banking system (АБС) */
  abs_reference?: string;
  /** Корреспондентский счёт банка (этап 9) */
  correspondent_account?: string;
  tariff_plan?: string;
  agreements?: AccountAgreements;
  /** Профиль постоянного мониторинга, привязанный к счёту (этап 10) */
  monitoring_profile_id?: string;
  created_at: string;
}

/** Immutable append-only audit log entry. */
export interface AuditEvent {
  id: string;
  tenant_id: string;
  /** Dot-namespaced event name, e.g. 'application.submitted', 'document.uploaded' */
  event_type: string;
  /** Identifier of the actor: a user ID, 'system', or an AI agent name */
  actor_id: string;
  /** Role of the actor, e.g. 'operator', 'system', 'ai_agent' */
  actor_role: string;
  /** Domain entity type that was affected, e.g. 'application', 'document', 'decision' */
  resource_type: string;
  /** Identifier of the affected entity */
  resource_id: string;
  /** Domain-specific event payload; schema varies by event_type */
  payload: Record<string, unknown>;
  /** IP address of the request origin — null for system-generated events */
  ip_address?: string;
  created_at: string;
}

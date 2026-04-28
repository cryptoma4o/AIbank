// Code generated from schema.json. DO NOT EDIT.
// Source: schema.json (https://aibank.io/schemas/domain-model/v1.0.0/schema.json)

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

/** Semantic category of an uploaded document */
export type DocumentType = "passport" | "inn_certificate" | "ogrn_certificate" | "charter" | "beneficial_owners_registry" | "ceo_appointment_order" | "bank_account_application" | "power_of_attorney" | "financial_statements" | "other";

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

/** Functional type of a bank account */
export type AccountType = "settlement" | "deposit" | "loan";

export const AccountTypeValues = [
  "settlement",
  "deposit",
  "loan",
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
  /** Registered legal address as plain text */
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

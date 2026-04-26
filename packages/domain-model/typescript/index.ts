/**
 * Canonical domain model for the AIbank white-label digital onboarding platform.
 *
 * This file is hand-written and is the authoritative TypeScript representation of
 * schema.json. It must be kept in sync with the JSON Schema manually.
 *
 * ID format: {prefix}_{ULID}  e.g. tnt_01ARZ3NDEKTSV4RRFFQ69G5FAV
 * Money:     always number (integer kopecks), never float
 * Dates:     UTC ISO 8601 strings
 */

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

export type ApplicationStatus =
  | "draft"
  | "documents_pending"
  | "validation_in_progress"
  | "risk_scoring"
  | "manual_review"
  | "auto_approved"
  | "rejected"
  | "account_opening"
  | "completed"
  | "cancelled";

export type DocumentType =
  | "passport"
  | "inn_certificate"
  | "ogrn_certificate"
  | "charter"
  | "beneficial_owners_registry"
  | "ceo_appointment_order"
  | "bank_account_application"
  | "power_of_attorney"
  | "financial_statements"
  | "other";

export type DocumentStatus = "uploaded" | "processing" | "verified" | "rejected";

export type LegalForm = "ooo" | "ao" | "pao" | "ip" | "zao";

export type LegalEntityStatus =
  | "active"
  | "liquidating"
  | "liquidated"
  | "reorganizing";

export type RiskLevel = "low" | "medium" | "high" | "critical";

export type Recommendation = "approve" | "manual_review" | "reject";

export type DecisionType =
  | "auto_approved"
  | "auto_rejected"
  | "manual_approved"
  | "manual_rejected"
  | "escalated";

export type Currency = "RUB" | "USD" | "EUR";

export type AccountType = "settlement" | "deposit" | "loan";

export type SignatureType = "ukep" | "pep" | "none";

export type UBONodeType = "person" | "legal_entity";

// ---------------------------------------------------------------------------
// Sub-entities / value objects
// ---------------------------------------------------------------------------

/** A single factor contributing to a risk score. */
export interface RiskFactor {
  /** Machine-readable factor identifier. */
  name: string;
  /** Observed value of the factor (any scalar). */
  value: unknown;
  /** Signed integer contribution to the overall score (positive = higher risk). */
  impact: number;
  /** Human-readable explanation of how this factor affects the score. */
  explanation: string;
}

/** A node (person or legal entity) in the beneficial ownership graph. */
export interface UBONode {
  /** Node identifier within the graph. */
  id: string;
  /** Reference to Person entity — present when node_type is "person". */
  person_id: string | null;
  /** Reference to LegalEntity — present when node_type is "legal_entity". */
  legal_entity_id: string | null;
  /** Display name of the node. */
  name: string;
  node_type: UBONodeType;
  /** Direct ownership stake as a percentage (0–100). */
  direct_stake: number;
  /** Computed effective (indirect + direct) ownership stake as a percentage. */
  effective_stake: number;
  /** True when effective_stake >= 25% (ultimate beneficial owner threshold). */
  is_ubo: boolean;
}

/** A directed ownership edge between two nodes in the beneficial ownership graph. */
export interface UBOEdge {
  from_node_id: string;
  to_node_id: string;
  /** Ownership stake on this edge as a percentage (0–100). */
  stake: number;
  /** Supporting document that evidences this ownership stake. */
  document_id: string | null;
}

// ---------------------------------------------------------------------------
// Core entities
// ---------------------------------------------------------------------------

/**
 * Tenant — a bank that uses the AIbank platform.
 * Root multi-tenant entity; every other entity belongs to exactly one Tenant.
 * ID prefix: tnt_
 */
export interface Tenant {
  id: string;
  name: string;
  /** Kebab-case unique identifier used in URLs and config paths. */
  slug: string;
  status: "active" | "suspended" | "trial";
  config_version: string;
  created_at: string;
  updated_at: string;
}

/**
 * Application — a single onboarding application.
 * Core state-machine entity — one per legal entity from draft to account opening.
 * ID prefix: app_
 */
export interface Application {
  id: string;
  tenant_id: string;
  client_id: string | null;
  legal_entity_id: string | null;
  status: ApplicationStatus;
  /** Temporal workflow execution ID tracking this application. */
  workflow_id: string | null;
  created_at: string;
  updated_at: string;
  submitted_at: string | null;
  decided_at: string | null;
  /** Opened account — null until status is "completed". */
  account_id: string | null;
}

/**
 * Person — any natural person in the system: applicant, director, founder, UBO, signatory.
 * ID prefix: per_
 */
export interface Person {
  id: string;
  tenant_id: string;
  last_name: string;
  first_name: string;
  /** Patronymic (отчество) — optional. */
  middle_name: string | null;
  birth_date: string;
  /** Individual taxpayer number — exactly 12 digits. */
  inn: string;
  /** Insurance number — optional. */
  snils: string | null;
  /** Russian passport series — 4 digits. */
  passport_series: string;
  /** Russian passport number — 6 digits. */
  passport_number: string;
  passport_issued_by: string;
  passport_issued_at: string;
  created_at: string;
}

/**
 * LegalEntity — a Russian legal entity (ООО, АО, etc.).
 * ID prefix: le_
 */
export interface LegalEntity {
  id: string;
  tenant_id: string;
  full_name: string;
  short_name: string;
  /** Taxpayer identification number — exactly 10 digits for legal entities. */
  inn: string;
  /** Primary state registration number — exactly 13 digits. */
  ogrn: string;
  /** Tax registration reason code — 9 digits. Null for individual entrepreneurs (IP). */
  kpp: string | null;
  legal_form: LegalForm;
  legal_address: string;
  actual_address: string | null;
  okved_primary: string;
  okved_secondary: string[];
  registration_date: string;
  status: LegalEntityStatus;
  ceo_person_id: string;
  created_at: string;
  updated_at: string;
}

/**
 * UBOGraph — computed beneficial ownership graph for one legal entity.
 * ID prefix: ubn_
 */
export interface UBOGraph {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  nodes: UBONode[];
  edges: UBOEdge[];
  computed_at: string;
}

/**
 * Document — a document uploaded by the applicant or retrieved from an external source.
 * ID prefix: doc_
 */
export interface Document {
  id: string;
  tenant_id: string;
  application_id: string;
  document_type: DocumentType;
  filename: string;
  mime_type: string;
  size_bytes: number;
  s3_key: string;
  checksum_sha256: string;
  status: DocumentStatus;
  /** Key-value pairs extracted by OCR/ML. Null when not yet processed. */
  extracted_fields: Record<string, unknown> | null;
  /** Overall OCR confidence score (0–1). Null when not yet processed. */
  ocr_confidence: number | null;
  signed_at: string | null;
  signature_type: SignatureType;
  created_at: string;
  updated_at: string;
}

/**
 * RiskAssessment — ML-based risk scoring result for an onboarding application.
 * ID prefix: ris_
 */
export interface RiskAssessment {
  id: string;
  tenant_id: string;
  application_id: string;
  /** Risk score from 0 (lowest risk) to 100 (highest risk). */
  score: number;
  risk_level: RiskLevel;
  recommendation: Recommendation;
  factors: RiskFactor[];
  /** Semver of the risk model that produced this assessment. */
  model_version: string;
  assessed_at: string;
}

/**
 * Decision — the final underwriting decision on an onboarding application.
 * ID prefix: dec_
 */
export interface Decision {
  id: string;
  tenant_id: string;
  application_id: string;
  decision_type: DecisionType;
  /** User ID of the operator for manual decisions. Null for automated decisions. */
  actor_id: string | null;
  reason: string;
  risk_assessment_id: string;
  created_at: string;
}

/**
 * Account — a bank account opened for the legal entity after an approved application.
 * ID prefix: acc_
 */
export interface Account {
  id: string;
  tenant_id: string;
  application_id: string;
  legal_entity_id: string;
  /** 20-digit account number per the Russian bank account plan. */
  account_number: string;
  /** Bank Identification Code — exactly 9 digits. */
  bik: string;
  bank_name: string;
  currency: Currency;
  account_type: AccountType;
  opened_at: string;
  /** Internal reference identifier in the bank's core banking system (АБС). */
  abs_reference: string | null;
  created_at: string;
}

/**
 * AuditEvent — immutable append-only audit log entry.
 * ID prefix: cev_
 */
export interface AuditEvent {
  id: string;
  tenant_id: string;
  /** Dot-namespaced event name, e.g. 'application.submitted', 'document.uploaded'. */
  event_type: string;
  actor_id: string;
  actor_role: string;
  resource_type: string;
  resource_id: string;
  /** Domain-specific event payload; schema varies by event_type. */
  payload: Record<string, unknown>;
  ip_address: string | null;
  created_at: string;
}

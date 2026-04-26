"""
Canonical domain model for the AIbank white-label digital onboarding platform.

This file is hand-written and is the authoritative Python representation of
schema.json. It must be kept in sync with the JSON Schema manually.

ID format: {prefix}_{ULID}  e.g. tnt_01ARZ3NDEKTSV4RRFFQ69G5FAV
Money:     always int (kopecks), never float
Dates:     UTC ISO 8601 strings
"""
from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any, Optional


# ---------------------------------------------------------------------------
# Enums
# ---------------------------------------------------------------------------


class ApplicationStatus(StrEnum):
    DRAFT = "draft"
    DOCUMENTS_PENDING = "documents_pending"
    VALIDATION_IN_PROGRESS = "validation_in_progress"
    RISK_SCORING = "risk_scoring"
    MANUAL_REVIEW = "manual_review"
    AUTO_APPROVED = "auto_approved"
    REJECTED = "rejected"
    ACCOUNT_OPENING = "account_opening"
    COMPLETED = "completed"
    CANCELLED = "cancelled"


class DocumentType(StrEnum):
    PASSPORT = "passport"
    INN_CERTIFICATE = "inn_certificate"
    OGRN_CERTIFICATE = "ogrn_certificate"
    CHARTER = "charter"
    BENEFICIAL_OWNERS_REGISTRY = "beneficial_owners_registry"
    CEO_APPOINTMENT_ORDER = "ceo_appointment_order"
    BANK_ACCOUNT_APPLICATION = "bank_account_application"
    POWER_OF_ATTORNEY = "power_of_attorney"
    FINANCIAL_STATEMENTS = "financial_statements"
    OTHER = "other"


class DocumentStatus(StrEnum):
    UPLOADED = "uploaded"
    PROCESSING = "processing"
    VERIFIED = "verified"
    REJECTED = "rejected"


class LegalForm(StrEnum):
    OOO = "ooo"
    AO = "ao"
    PAO = "pao"
    IP = "ip"
    ZAO = "zao"


class LegalEntityStatus(StrEnum):
    ACTIVE = "active"
    LIQUIDATING = "liquidating"
    LIQUIDATED = "liquidated"
    REORGANIZING = "reorganizing"


class RiskLevel(StrEnum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class Recommendation(StrEnum):
    APPROVE = "approve"
    MANUAL_REVIEW = "manual_review"
    REJECT = "reject"


class DecisionType(StrEnum):
    AUTO_APPROVED = "auto_approved"
    AUTO_REJECTED = "auto_rejected"
    MANUAL_APPROVED = "manual_approved"
    MANUAL_REJECTED = "manual_rejected"
    ESCALATED = "escalated"


class Currency(StrEnum):
    RUB = "RUB"
    USD = "USD"
    EUR = "EUR"


class AccountType(StrEnum):
    SETTLEMENT = "settlement"
    DEPOSIT = "deposit"
    LOAN = "loan"


class SignatureType(StrEnum):
    UKEP = "ukep"
    PEP = "pep"
    NONE = "none"


class UBONodeType(StrEnum):
    PERSON = "person"
    LEGAL_ENTITY = "legal_entity"


class TenantStatus(StrEnum):
    ACTIVE = "active"
    SUSPENDED = "suspended"
    TRIAL = "trial"


# ---------------------------------------------------------------------------
# Sub-entities / value objects
# ---------------------------------------------------------------------------


@dataclass
class RiskFactor:
    """A single factor contributing to a risk score."""

    name: str
    """Machine-readable factor identifier."""

    value: Any
    """Observed value of the factor (any scalar)."""

    impact: int
    """Signed integer contribution to the overall score (positive = higher risk)."""

    explanation: str
    """Human-readable explanation of how this factor affects the score."""


@dataclass
class UBONode:
    """A node (person or legal entity) in the beneficial ownership graph."""

    id: str
    """Node identifier within the graph."""

    name: str
    """Display name of the node."""

    node_type: UBONodeType

    direct_stake: float
    """Direct ownership stake as a percentage (0–100)."""

    effective_stake: float
    """Computed effective (indirect + direct) ownership stake as a percentage."""

    is_ubo: bool
    """True when effective_stake >= 25% (ultimate beneficial owner threshold)."""

    person_id: Optional[str] = None
    """Reference to Person entity — present when node_type is PERSON."""

    legal_entity_id: Optional[str] = None
    """Reference to LegalEntity — present when node_type is LEGAL_ENTITY."""


@dataclass
class UBOEdge:
    """A directed ownership edge between two nodes in the beneficial ownership graph."""

    from_node_id: str
    to_node_id: str
    stake: float
    """Ownership stake on this edge as a percentage (0–100)."""

    document_id: Optional[str] = None
    """Supporting document that evidences this ownership stake."""


# ---------------------------------------------------------------------------
# Core entities
# ---------------------------------------------------------------------------


@dataclass
class Tenant:
    """
    A bank that uses the AIbank platform.
    Root multi-tenant entity — every other entity belongs to exactly one Tenant.
    ID prefix: tnt_
    """

    id: str
    name: str
    slug: str
    """Kebab-case unique identifier used in URLs and config paths."""

    status: TenantStatus
    config_version: str
    created_at: str
    updated_at: str


@dataclass
class Application:
    """
    A single onboarding application. Core state-machine entity.
    ID prefix: app_
    """

    id: str
    tenant_id: str
    status: ApplicationStatus
    created_at: str
    updated_at: str

    client_id: Optional[str] = None
    legal_entity_id: Optional[str] = None
    workflow_id: Optional[str] = None
    """Temporal workflow execution ID tracking this application."""

    submitted_at: Optional[str] = None
    decided_at: Optional[str] = None
    account_id: Optional[str] = None
    """Opened account — None until status is COMPLETED."""


@dataclass
class Person:
    """
    Any natural person in the system: applicant, director, founder, UBO, signatory.
    ID prefix: per_
    """

    id: str
    tenant_id: str
    last_name: str
    first_name: str
    birth_date: str
    inn: str
    """Individual taxpayer number — exactly 12 digits."""

    passport_series: str
    """Russian passport series — 4 digits."""

    passport_number: str
    """Russian passport number — 6 digits."""

    passport_issued_by: str
    passport_issued_at: str
    created_at: str

    middle_name: Optional[str] = None
    """Patronymic (отчество) — optional."""

    snils: Optional[str] = None
    """Insurance number — optional."""


@dataclass
class LegalEntity:
    """
    A Russian legal entity (ООО, АО, etc.) undergoing or having completed onboarding.
    ID prefix: le_
    """

    id: str
    tenant_id: str
    full_name: str
    short_name: str
    inn: str
    """Taxpayer identification number — exactly 10 digits for legal entities."""

    ogrn: str
    """Primary state registration number — exactly 13 digits."""

    legal_form: LegalForm
    legal_address: str
    okved_primary: str
    okved_secondary: list[str]
    registration_date: str
    status: LegalEntityStatus
    ceo_person_id: str
    created_at: str
    updated_at: str

    kpp: Optional[str] = None
    """Tax registration reason code — 9 digits. None for individual entrepreneurs (IP)."""

    actual_address: Optional[str] = None
    """Actual operating address — None if same as legal_address."""


@dataclass
class UBOGraph:
    """
    Computed beneficial ownership graph for one legal entity in the context of an application.
    ID prefix: ubn_
    """

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    nodes: list[UBONode]
    edges: list[UBOEdge]
    computed_at: str


@dataclass
class Document:
    """
    A document uploaded by the applicant or retrieved from an external source.
    ID prefix: doc_
    """

    id: str
    tenant_id: str
    application_id: str
    document_type: DocumentType
    filename: str
    mime_type: str
    size_bytes: int
    s3_key: str
    checksum_sha256: str
    status: DocumentStatus
    signature_type: SignatureType
    created_at: str
    updated_at: str

    extracted_fields: Optional[dict[str, Any]] = None
    """Key-value pairs extracted by OCR/ML. None when not yet processed."""

    ocr_confidence: Optional[float] = None
    """Overall OCR confidence score (0–1). None when not yet processed."""

    signed_at: Optional[str] = None
    """When the document was electronically signed — None if unsigned."""


@dataclass
class RiskAssessment:
    """
    ML-based risk scoring result for an onboarding application.
    ID prefix: ris_
    """

    id: str
    tenant_id: str
    application_id: str
    score: int
    """Risk score from 0 (lowest risk) to 100 (highest risk)."""

    risk_level: RiskLevel
    recommendation: Recommendation
    factors: list[RiskFactor]
    model_version: str
    """Semver of the risk model that produced this assessment."""

    assessed_at: str


@dataclass
class Decision:
    """
    The final underwriting decision on an onboarding application.
    ID prefix: dec_
    """

    id: str
    tenant_id: str
    application_id: str
    decision_type: DecisionType
    reason: str
    risk_assessment_id: str
    created_at: str

    actor_id: Optional[str] = None
    """User ID of the operator for manual decisions. None for automated decisions."""


@dataclass
class Account:
    """
    A bank account opened for the legal entity after an approved application.
    ID prefix: acc_
    """

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    account_number: str
    """20-digit account number per the Russian bank account plan."""

    bik: str
    """Bank Identification Code — exactly 9 digits."""

    bank_name: str
    currency: Currency
    account_type: AccountType
    opened_at: str
    created_at: str

    abs_reference: Optional[str] = None
    """Internal reference identifier in the bank's core banking system (АБС)."""


@dataclass
class AuditEvent:
    """
    Immutable append-only audit log entry.
    ID prefix: cev_
    """

    id: str
    tenant_id: str
    event_type: str
    """Dot-namespaced event name, e.g. 'application.submitted', 'document.uploaded'."""

    actor_id: str
    actor_role: str
    resource_type: str
    resource_id: str
    payload: dict[str, Any]
    """Domain-specific event payload; schema varies by event_type."""

    created_at: str

    ip_address: Optional[str] = None
    """IP address of the request origin — None for system-generated events."""

"""Code generated from schema.json. DO NOT EDIT.

Source: schema.json (https://aibank.io/schemas/domain-model/v1.0.0/schema.json)
"""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel


class ApplicationStatus(str, Enum):
    """State machine status for an onboarding application"""

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


class DocumentType(str, Enum):
    """Semantic category of an uploaded document"""

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


class DocumentStatus(str, Enum):
    """Processing state of a document"""

    UPLOADED = "uploaded"
    PROCESSING = "processing"
    VERIFIED = "verified"
    REJECTED = "rejected"


class LegalForm(str, Enum):
    """Legal organisational form of a Russian entity"""

    OOO = "ooo"
    AO = "ao"
    PAO = "pao"
    IP = "ip"
    ZAO = "zao"


class LegalEntityStatus(str, Enum):
    """Lifecycle status of a legal entity from the state register"""

    ACTIVE = "active"
    LIQUIDATING = "liquidating"
    LIQUIDATED = "liquidated"
    REORGANIZING = "reorganizing"


class RiskLevel(str, Enum):
    """Bucketed risk classification"""

    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class Recommendation(str, Enum):
    """Model recommendation based on risk score"""

    APPROVE = "approve"
    MANUAL_REVIEW = "manual_review"
    REJECT = "reject"


class DecisionType(str, Enum):
    """How and in which direction the decision was made"""

    AUTO_APPROVED = "auto_approved"
    AUTO_REJECTED = "auto_rejected"
    MANUAL_APPROVED = "manual_approved"
    MANUAL_REJECTED = "manual_rejected"
    ESCALATED = "escalated"


class Currency(str, Enum):
    """ISO 4217 currency code"""

    RUB = "RUB"
    USD = "USD"
    EUR = "EUR"


class AccountType(str, Enum):
    """Functional type of a bank account"""

    SETTLEMENT = "settlement"
    DEPOSIT = "deposit"
    LOAN = "loan"


class SignatureType(str, Enum):
    """Type of electronic signature on a document"""

    UKEP = "ukep"
    PEP = "pep"
    NONE = "none"


class UBONodeType(str, Enum):
    """Type of node in the beneficial ownership graph"""

    PERSON = "person"
    LEGAL_ENTITY = "legal_entity"


class RiskFactor(BaseModel):
    """A single factor contributing to a risk score"""

    name: str
    value: Any
    impact: int
    explanation: str


class UBONode(BaseModel):
    """A node (person or legal entity) in the beneficial ownership graph"""

    id: str
    person_id: str | None = None
    legal_entity_id: str | None = None
    name: str
    node_type: UBONodeType
    direct_stake: float
    effective_stake: float
    is_ubo: bool


class UBOEdge(BaseModel):
    """A directed ownership edge between two nodes in the beneficial ownership graph"""

    from_node_id: str
    to_node_id: str
    stake: float
    document_id: str | None = None


class Tenant(BaseModel):
    """A bank that uses the AIbank platform. Root multi-tenant entity — every other entity belongs to exactly one Tenant."""

    id: str
    name: str
    slug: str
    status: str
    config_version: str
    created_at: datetime
    updated_at: datetime


class Application(BaseModel):
    """A single onboarding application. Core state-machine entity — one per legal entity from draft to account opening or rejection."""

    id: str
    tenant_id: str
    client_id: str | None = None
    legal_entity_id: str | None = None
    status: ApplicationStatus
    workflow_id: str | None = None
    created_at: datetime
    updated_at: datetime
    submitted_at: datetime | None = None
    decided_at: datetime | None = None
    account_id: str | None = None


class Person(BaseModel):
    """Any natural person in the system: applicant, director, founder, UBO, signatory."""

    id: str
    tenant_id: str
    last_name: str
    first_name: str
    middle_name: str | None = None
    birth_date: datetime
    inn: str
    snils: str | None = None
    passport_series: str
    passport_number: str
    passport_issued_by: str
    passport_issued_at: datetime
    created_at: datetime


class LegalEntity(BaseModel):
    """A Russian legal entity (ООО, АО, etc.) undergoing or having completed onboarding."""

    id: str
    tenant_id: str
    full_name: str
    short_name: str
    inn: str
    ogrn: str
    kpp: str | None = None
    legal_form: LegalForm
    legal_address: str
    actual_address: str | None = None
    okved_primary: str
    okved_secondary: list[str]
    registration_date: datetime
    status: LegalEntityStatus
    ceo_person_id: str
    created_at: datetime
    updated_at: datetime


class UBOGraph(BaseModel):
    """Computed beneficial ownership graph for one legal entity in the context of an application."""

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    nodes: list[UBONode]
    edges: list[UBOEdge]
    computed_at: datetime


class Document(BaseModel):
    """A document uploaded by the applicant or retrieved from an external source."""

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
    extracted_fields: dict[str, Any] | None = None
    ocr_confidence: float | None = None
    signed_at: datetime | None = None
    signature_type: SignatureType
    created_at: datetime
    updated_at: datetime


class RiskAssessment(BaseModel):
    """ML-based risk scoring result for an onboarding application."""

    id: str
    tenant_id: str
    application_id: str
    score: int
    risk_level: RiskLevel
    recommendation: Recommendation
    factors: list[RiskFactor]
    model_version: str
    assessed_at: datetime


class Decision(BaseModel):
    """The final underwriting decision on an onboarding application."""

    id: str
    tenant_id: str
    application_id: str
    decision_type: DecisionType
    actor_id: str | None = None
    reason: str
    risk_assessment_id: str
    created_at: datetime


class Account(BaseModel):
    """A bank account opened for the legal entity after an approved application."""

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    account_number: str
    bik: str
    bank_name: str
    currency: Currency
    account_type: AccountType
    opened_at: datetime
    abs_reference: str | None = None
    created_at: datetime


class AuditEvent(BaseModel):
    """Immutable append-only audit log entry."""

    id: str
    tenant_id: str
    event_type: str
    actor_id: str
    actor_role: str
    resource_type: str
    resource_id: str
    payload: dict[str, Any]
    ip_address: str | None = None
    created_at: datetime


__all__ = [
    "ApplicationStatus",
    "DocumentType",
    "DocumentStatus",
    "LegalForm",
    "LegalEntityStatus",
    "RiskLevel",
    "Recommendation",
    "DecisionType",
    "Currency",
    "AccountType",
    "SignatureType",
    "UBONodeType",
    "RiskFactor",
    "UBONode",
    "UBOEdge",
    "Tenant",
    "Application",
    "Person",
    "LegalEntity",
    "UBOGraph",
    "Document",
    "RiskAssessment",
    "Decision",
    "Account",
    "AuditEvent",
]

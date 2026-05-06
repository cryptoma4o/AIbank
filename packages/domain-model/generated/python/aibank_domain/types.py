"""Code generated from schema.json. DO NOT EDIT.

Source: schema.json (https://aibank.io/schemas/domain-model/v1.1.0/schema.json)
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
    """Semantic category of an uploaded document. Расширено в v1.1.0 для покрытия этапа 6 формы (см. docs/onboarding-form-spec.md). Add-only: старые значения сохранены."""

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
    EGRUL_RECORD_SHEET = "egrul_record_sheet"
    EIO_APPOINTMENT_PROTOCOL = "eio_appointment_protocol"
    EIO_ORDER = "eio_order"
    SIGNATURE_CARD = "signature_card"
    LICENSE = "license"
    ADDRESS_CONFIRMATION = "address_confirmation"
    TAX_DECLARATION = "tax_declaration"
    TAX_CLEARANCE_CERTIFICATE = "tax_clearance_certificate"
    OWNERSHIP_CHAIN_DIAGRAM = "ownership_chain_diagram"
    FATCA_W8_FORM = "fatca_w8_form"
    FATCA_W9_FORM = "fatca_w9_form"
    MIGRATION_CARD = "migration_card"
    RESIDENCE_PERMIT = "residence_permit"
    FOREIGN_PASSPORT = "foreign_passport"


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
    """Functional type of a bank account. Add-only: расширено в v1.1.0 для покрытия этапа 9 (settlement/special/foreign_currency/escrow/nominal — см. docs/onboarding-form-spec.md §9)."""

    SETTLEMENT = "settlement"
    DEPOSIT = "deposit"
    LOAN = "loan"
    SPECIAL = "special"
    FOREIGN_CURRENCY = "foreign_currency"
    ESCROW = "escrow"
    NOMINAL = "nominal"


class SignatureType(str, Enum):
    """Type of electronic signature on a document"""

    UKEP = "ukep"
    PEP = "pep"
    NONE = "none"


class UBONodeType(str, Enum):
    """Type of node in the beneficial ownership graph"""

    PERSON = "person"
    LEGAL_ENTITY = "legal_entity"


class TaxRegime(str, Enum):
    """Tax regime applicable to a Russian legal entity / IP"""

    OSN = "osn"
    USN_INCOME = "usn_income"
    USN_INCOME_MINUS_EXPENSES = "usn_income_minus_expenses"
    PSN = "psn"
    ESHN = "eshn"
    NPD = "npd"


class AuthorityBasis(str, Enum):
    """Legal basis on which the representative acts on behalf of the legal entity (этап 4)"""

    CHARTER = "charter"
    PROTOCOL = "protocol"
    POWER_OF_ATTORNEY = "power_of_attorney"
    ORDER = "order"


class IDDocumentType(str, Enum):
    """Type of identity document presented by a representative / UBO"""

    PASSPORT_RU = "passport_ru"
    PASSPORT_FOREIGN = "passport_foreign"
    NATIONAL_PASSPORT = "national_passport"
    REFUGEE_CERTIFICATE = "refugee_certificate"


class PDLCategory(str, Enum):
    """Politically-exposed-person category — uses PEP terminology in regulatory texts"""

    FOREIGN = "foreign"
    RUSSIAN = "russian"
    INTERNATIONAL_ORGANIZATION = "international_organization"


class PDLRelation(str, Enum):
    """How the person relates to a politically-exposed-person status"""

    SELF = "self"
    RELATIVE = "relative"
    REPRESENTATIVE = "representative"


class BusinessRiskCategory(str, Enum):
    """AML risk category of the declared OKVED activity (этап 3)"""

    LOW_RISK = "low_risk"
    MEDIUM_RISK = "medium_risk"
    HIGH_RISK = "high_risk"


class FundsSourceCategory(str, Enum):
    """Declared origin of operating funds"""

    REVENUE = "revenue"
    FOUNDER_CONTRIBUTION = "founder_contribution"
    LOAN = "loan"
    INVESTMENTS = "investments"
    OTHER = "other"


class ControlBasis(str, Enum):
    """Basis on which a person controls the legal entity (UBO — этап 5)"""

    CAPITAL_SHARE = "capital_share"
    CONTRACT = "contract"
    OTHER_DECISION_RIGHT = "other_decision_right"


class ScreeningMatchLevel(str, Enum):
    """Screening match outcome against a watch list"""

    MATCH = "match"
    PARTIAL_MATCH = "partial_match"
    NO_MATCH = "no_match"


class AdverseMediaCategory(str, Enum):
    """Category of adverse media hit"""

    FRAUD = "fraud"
    MONEY_LAUNDERING = "money_laundering"
    TERRORISM_FINANCING = "terrorism_financing"
    SANCTIONS = "sanctions"
    CORRUPTION = "corruption"
    OTHER = "other"


class RelationshipType(str, Enum):
    """Counterparty relationship — постоянный / разовый"""

    REGULAR = "regular"
    ONE_OFF = "one_off"


class KYCRefreshTrigger(str, Enum):
    """Event that triggers re-execution of KYC procedures"""

    SCHEDULED = "scheduled"
    CEO_CHANGE = "ceo_change"
    UBO_CHANGE = "ubo_change"
    OWNERSHIP_CHANGE = "ownership_change"
    ADDRESS_CHANGE = "address_change"
    LICENSE_EXPIRY = "license_expiry"
    TRANSACTION_THRESHOLD_EXCEEDED = "transaction_threshold_exceeded"
    REGULATOR_ALERT = "regulator_alert"


class SigningMethod(str, Enum):
    """Method used to sign account-opening agreements"""

    UKEP = "ukep"
    SMS_CODE = "sms_code"
    HANDWRITTEN = "handwritten"


class RemoteBankingChannel(str, Enum):
    """Channel through which DBO (remote banking) is accessed"""

    WEB = "web"
    MOBILE = "mobile"
    API = "api"


class ZSKLight(str, Enum):
    """ЗСК (Знай своего клиента) traffic-light category from CBR"""

    GREEN = "green"
    YELLOW = "yellow"
    RED = "red"
    UNKNOWN = "unknown"


class PrequalificationDecision(str, Enum):
    """Aggregated decision after the pre-qualification stage (этап 1)"""

    PROCEED = "proceed"
    MANUAL_REVIEW = "manual_review"
    REJECT = "reject"


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


class StructuredAddress(BaseModel):
    """Structured address per ФИАС, used in legal/actual/postal address blocks (этап 2) and для физлица (этап 4)."""

    country_code: str
    postal_code: str | None = None
    region_code: str | None = None
    region_name: str | None = None
    city: str
    street: str | None = None
    building: str | None = None
    office: str | None = None
    fias_id: str | None = None


class MoneyAmount(BaseModel):
    """Monetary amount in a specific currency"""

    amount: float
    currency: str


class ContactInfo(BaseModel):
    """Contact details for a legal entity (этап 2)"""

    phone: str | None = None
    email: str | None = None
    website: str | None = None


class LicenseInfo(BaseModel):
    """Licence held by a legal entity (этап 2)"""

    number: str
    issue_date: str
    expiry_date: str | None = None
    issuer: str
    activity_type: str


class SROMembership(BaseModel):
    """Self-regulating organisation membership (этап 2)"""

    name: str
    reg_number: str
    join_date: str


class Counterparty(BaseModel):
    """Top supplier or top buyer counterparty (этап 3)"""

    name: str
    inn: str | None = None
    country: str
    share_percent: float
    relationship_type: RelationshipType


class OperationalModel(BaseModel):
    """Planned operational model (этап 3)"""

    geography: list[str] | None = None
    monthly_turnover_planned: MoneyAmount | None = None
    annual_turnover_planned: MoneyAmount | None = None
    cash_share_percent: float | None = None
    foreign_economic_activity: bool | None = None
    foreign_countries: list[str] | None = None
    currency_operations: list[str] | None = None


class FundsSource(BaseModel):
    """Declared source of funds (этап 3)"""

    category: FundsSourceCategory
    description: str | None = None


class IDDocument(BaseModel):
    """Identity document for a representative or UBO (этап 4/5)"""

    doc_type: IDDocumentType
    series: str | None = None
    number: str
    issue_date: str | None = None
    expiry_date: str | None = None
    issued_by: str | None = None
    department_code: str | None = None


class AuthorityInfo(BaseModel):
    """Powers of a representative (этап 4)"""

    position: str
    authority_basis: AuthorityBasis
    authority_doc_number: str | None = None
    authority_doc_date: str | None = None
    signature_sample_doc_id: str | None = None


class ForeignerInfo(BaseModel):
    """Additional details required for non-RU citizens (этап 4)"""

    migration_card_number: str | None = None
    migration_card_issued_at: str | None = None
    migration_card_expires_at: str | None = None
    residence_doc_type: str | None = None
    residence_doc_number: str | None = None
    residence_doc_issued_at: str | None = None
    residence_doc_expires_at: str | None = None


class PDLDeclaration(BaseModel):
    """Politically-exposed-person declaration (этап 4)"""

    is_pdl: bool
    category: PDLCategory | None = None
    position: str | None = None
    relation: PDLRelation | None = None


class OwnershipChainLink(BaseModel):
    """Single hop in the ownership chain leading to a UBO (этап 5)"""

    level: int
    entity_name: str
    entity_inn_or_reg_number: str | None = None
    country: str | None = None
    share_percent: float


class FATCADeclaration(BaseModel):
    """FATCA / CRS tax-residency declaration (этап 5)"""

    tax_residency_countries: list[str] | None = None
    tin_per_country: list[dict[str, Any]] | None = None
    us_person: bool
    form_doc_id: str | None = None


class ScreeningResult(BaseModel):
    """Outcome of a single watch-list screen (этап 7)"""

    list_name: str
    match_level: ScreeningMatchLevel
    score: float | None = None
    matched_entity_name: str | None = None
    checked_at: datetime | None = None


class AdverseMediaHit(BaseModel):
    """Negative-news article matching the subject (этап 7)"""

    category: AdverseMediaCategory
    title: str
    url: str
    source: str | None = None
    published_at: str
    summary: str | None = None


class AntiFraudSignals(BaseModel):
    """Submission-time anti-fraud signals (этап 7)"""

    device_fingerprint: str | None = None
    ip_address: str | None = None
    geolocation: dict[str, Any] | None = None


class MonitoringRule(BaseModel):
    """375-П / per-tenant monitoring rule applied to the account (этап 10)"""

    code: str
    description: str
    params: dict[str, Any] | None = None


class TransactionLimits(BaseModel):
    """Per-operation-type limits applied at account opening (этап 8/9)"""

    daily_outgoing: MoneyAmount | None = None
    daily_cash_withdrawal: MoneyAmount | None = None
    monthly_outgoing: MoneyAmount | None = None
    single_transaction_max: MoneyAmount | None = None


class AccountAgreements(BaseModel):
    """Bundle of consents and agreements signed at account opening (этап 9)"""

    agreement_acceptance: bool
    agreement_accepted_at: datetime
    dbo_agreement: bool | None = None
    dbo_channels: list[RemoteBankingChannel] | None = None
    edo_agreement: bool | None = None
    personal_data_consent: bool
    signing_method: SigningMethod
    ukep_certificate_serial: str | None = None


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
    prequalification_check_id: str | None = None
    legal_entity_profile_id: str | None = None
    activity_id: str | None = None
    screening_set_id: str | None = None
    monitoring_profile_id: str | None = None


class Person(BaseModel):
    """Any natural person in the system: applicant, director, founder, UBO, signatory."""

    id: str
    tenant_id: str
    last_name: str
    first_name: str
    middle_name: str | None = None
    birth_date: datetime
    birth_place: str | None = None
    citizenship: list[str] | None = None
    inn: str
    snils: str | None = None
    passport_series: str
    passport_number: str
    passport_issued_by: str
    passport_issued_at: datetime
    id_document: IDDocument | None = None
    registration_address: StructuredAddress | None = None
    actual_address: StructuredAddress | None = None
    foreigner_info: ForeignerInfo | None = None
    pdl_declaration: PDLDeclaration | None = None
    fatca_declaration: FATCADeclaration | None = None
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
    profile_id: str | None = None
    created_at: datetime
    updated_at: datetime


class LegalEntityProfile(BaseModel):
    """Расширенная анкета юрлица (этап 2). Дополняет LegalEntity полями, которые собираются при онбординге, но не входят в core-сущность ЕГРЮЛ. Связь — через LegalEntity.profile_id."""

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    opf_code: str | None = None
    registration_authority: str | None = None
    authorized_capital: MoneyAmount | None = None
    legal_address_struct: StructuredAddress | None = None
    actual_address_struct: StructuredAddress | None = None
    actual_same_as_legal: bool | None = None
    postal_address_struct: StructuredAddress | None = None
    postal_same_as_legal: bool | None = None
    okved_main_v2: str | None = None
    okved_additional_v2: list[str] | None = None
    licenses: list[LicenseInfo] | None = None
    sro_membership: list[SROMembership] | None = None
    contacts: ContactInfo | None = None
    employees_count: int | None = None
    revenue_last_year: MoneyAmount | None = None
    tax_regime: TaxRegime | None = None
    created_at: datetime
    updated_at: datetime


class ApplicationActivity(BaseModel):
    """AML-сведения о деятельности заявителя (этап 3). Один-к-одному к Application."""

    id: str
    tenant_id: str
    application_id: str
    business_description: str
    business_category: BusinessRiskCategory
    top_suppliers: list[Counterparty] | None = None
    top_buyers: list[Counterparty] | None = None
    operational_model: OperationalModel | None = None
    funds_source: FundsSource
    created_at: datetime
    updated_at: datetime


class Representative(BaseModel):
    """Единоличный исполнительный орган (ЕИО) или иной представитель юрлица (этап 4). Person хранит персональные данные, Representative — связь с заявкой и полномочия."""

    id: str
    tenant_id: str
    application_id: str
    legal_entity_id: str
    person_id: str
    authority: AuthorityInfo
    is_primary: bool
    is_signatory: bool | None = None
    created_at: datetime
    updated_at: datetime


class PrequalificationCheck(BaseModel):
    """Результат предварительной проверки по ИНН/ОГРН/short_name (этап 1) — агрегирует ответы внешних источников и финальное решение proceed/manual/reject."""

    id: str
    tenant_id: str
    application_id: str
    inn: str
    ogrn: str
    short_name_hint: str | None = None
    egrul_status: LegalEntityStatus | None = None
    egrul_registration_date: str | None = None
    egrul_address: str | None = None
    egrul_ceo_name: str | None = None
    egrul_founders_summary: str | None = None
    zsk_light: ZSKLight | None = None
    zsk_assigned_at: str | None = None
    p639_present: bool | None = None
    p639_reason: str | None = None
    sanctions_results: list[ScreeningResult] | None = None
    rosfinmon_present: bool | None = None
    decision: PrequalificationDecision
    decision_reason: str | None = None
    checked_at: datetime
    created_at: datetime


class ScreeningResultSet(BaseModel):
    """Набор сводных AML-проверок по заявке (этап 7). Включает санкции/PEP/adverse media по всем лицам + фрод-сигналы."""

    id: str
    tenant_id: str
    application_id: str
    sanctions_results: list[ScreeningResult] | None = None
    pep_results: list[ScreeningResult] | None = None
    adverse_media_hits: list[AdverseMediaHit] | None = None
    okved_consistency_score: float | None = None
    turnover_realism_score: float | None = None
    anti_fraud_signals: AntiFraudSignals | None = None
    performed_at: datetime
    created_at: datetime


class MonitoringProfile(BaseModel):
    """Параметры постоянного мониторинга открытого счёта (этап 10). Создаётся одновременно с Account."""

    id: str
    tenant_id: str
    application_id: str
    account_id: str
    review_frequency_months: int
    next_review_date: str
    monitoring_rules: list[MonitoringRule] | None = None
    kyc_refresh_triggers: list[KYCRefreshTrigger] | None = None
    transaction_limits: TransactionLimits | None = None
    notification_channels: list[str] | None = None
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
    ownership_chains: list[dict[str, Any]] | None = None
    no_ubo_reason: str | None = None
    eio_as_ubo_confirmation: bool | None = None
    diagram_doc_id: str | None = None
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
    issue_date: str | None = None
    expiry_date: str | None = None
    signed_by_person_id: str | None = None
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
    monitoring_profile_id: str | None = None
    transaction_limits: TransactionLimits | None = None
    screening_set_id: str | None = None
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
    correspondent_account: str | None = None
    tariff_plan: str | None = None
    agreements: AccountAgreements | None = None
    monitoring_profile_id: str | None = None
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
    "TaxRegime",
    "AuthorityBasis",
    "IDDocumentType",
    "PDLCategory",
    "PDLRelation",
    "BusinessRiskCategory",
    "FundsSourceCategory",
    "ControlBasis",
    "ScreeningMatchLevel",
    "AdverseMediaCategory",
    "RelationshipType",
    "KYCRefreshTrigger",
    "SigningMethod",
    "RemoteBankingChannel",
    "ZSKLight",
    "PrequalificationDecision",
    "RiskFactor",
    "UBONode",
    "UBOEdge",
    "StructuredAddress",
    "MoneyAmount",
    "ContactInfo",
    "LicenseInfo",
    "SROMembership",
    "Counterparty",
    "OperationalModel",
    "FundsSource",
    "IDDocument",
    "AuthorityInfo",
    "ForeignerInfo",
    "PDLDeclaration",
    "OwnershipChainLink",
    "FATCADeclaration",
    "ScreeningResult",
    "AdverseMediaHit",
    "AntiFraudSignals",
    "MonitoringRule",
    "TransactionLimits",
    "AccountAgreements",
    "Tenant",
    "Application",
    "Person",
    "LegalEntity",
    "LegalEntityProfile",
    "ApplicationActivity",
    "Representative",
    "PrequalificationCheck",
    "ScreeningResultSet",
    "MonitoringProfile",
    "UBOGraph",
    "Document",
    "RiskAssessment",
    "Decision",
    "Account",
    "AuditEvent",
]

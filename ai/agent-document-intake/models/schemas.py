"""Pydantic v2 schemas for agent-document-intake."""
from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class DocumentType(StrEnum):
    PASSPORT = "passport"
    EGRUL = "egrul"
    CHARTER = "charter"
    INN_CERTIFICATE = "inn_certificate"
    OGRN_CERTIFICATE = "ogrn_certificate"
    BANK_STATEMENT = "bank_statement"


class ExtractionRequest(BaseModel):
    document_id: str
    tenant_id: str
    document_type: DocumentType
    content_base64: str | None = None   # base64-encoded file bytes
    content_url: str | None = None      # presigned MinIO URL
    language: str = "ru"


class ExtractedField(BaseModel):
    name: str
    value: str
    confidence: float = Field(ge=0.0, le=1.0)


class ExtractionResult(BaseModel):
    document_id: str
    document_type: DocumentType
    fields: list[ExtractedField]
    raw_text: str
    model_used: str
    processing_ms: int
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)

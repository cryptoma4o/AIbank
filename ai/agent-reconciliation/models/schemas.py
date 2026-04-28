"""Pydantic schemas for the reconciliation agent."""
from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class Severity(StrEnum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"


class Discrepancy(BaseModel):
    field: str
    extracted_value: str
    registry_value: str
    severity: Severity = Severity.MEDIUM


class ReconciliationRequest(BaseModel):
    tenant_id: str
    application_id: str
    extracted_data: dict[str, Any] = Field(default_factory=dict)
    egrul_data: dict[str, Any] = Field(default_factory=dict)


class ReconciliationResult(BaseModel):
    matches: bool
    discrepancies: list[Discrepancy] = Field(default_factory=list)
    questions_for_client: list[str] = Field(default_factory=list)
    requires_review: bool = False
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)

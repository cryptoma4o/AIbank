from __future__ import annotations
from pydantic import BaseModel, Field


class ScoringRequest(BaseModel):
    application_id: str
    tenant_id: str
    inn: str
    ogrn: str
    okved: str
    company_age_years: int
    facts: dict = Field(default_factory=dict)


class ScoringResponse(BaseModel):
    application_id: str
    score: int                  # 0-100
    blocked: bool
    flags: list[str]
    fired_rules: list[str]
    explanation: str            # LLM-generated human-readable explanation
    recommendation: str         # "approve" | "manual_review" | "reject"
    model_used: str

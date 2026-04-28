"""Pydantic v2 schemas for agent-risk-scoring."""
from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field


class ScoringRequest(BaseModel):
    application_id: str
    tenant_id: str
    inn: str
    ogrn: str
    okved: str
    company_age_years: int
    facts: dict[str, Any] = Field(default_factory=dict)


class ScoringResponse(BaseModel):
    application_id: str
    score: int                  # 0-100 (higher = lower risk)
    blocked: bool
    flags: list[str]
    fired_rules: list[str]
    explanation: str            # may come from LLM or programmatic fallback
    explanation_source: str     # "llm" | "rule-based"
    recommendation: str         # "approve" | "manual_review" | "reject"
    model_used: str
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)

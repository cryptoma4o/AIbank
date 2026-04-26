from __future__ import annotations
from pydantic import BaseModel, Field


class ComplianceQuestion(BaseModel):
    question: str
    context: dict = Field(default_factory=dict)  # optional: application data for context-aware answers
    regulation_filter: str | None = None          # e.g. "115-fz", "375-p"
    top_k: int = 3


class ComplianceSource(BaseModel):
    doc_id: str
    title: str
    chunk: str
    source: str
    score: float


class ComplianceAnswer(BaseModel):
    question: str
    answer: str
    sources: list[ComplianceSource]
    confidence: str     # "high" | "medium" | "low"
    disclaimer: str
    model_used: str

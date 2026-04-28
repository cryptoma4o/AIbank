"""Pydantic v2 schemas for agent-compliance-assistant."""
from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


class AnswerRequest(BaseModel):
    tenant_id: str
    question: str
    context: dict[str, Any] = Field(
        default_factory=dict,
        description="Optional case context — application_id, client facts, regulation hints.",
    )
    top_k: int = 5
    source_type: str | None = None   # narrow RAG search to e.g. "law" / "regulation"


class Citation(BaseModel):
    doc_id: str
    snippet: str
    source: str
    source_type: str = "law"
    score: float = 0.0


Confidence = Literal["high", "medium", "low"]


class AnswerResponse(BaseModel):
    tenant_id: str
    question: str
    answer: str
    citations: list[Citation]
    confidence: Confidence
    requires_human_review: bool = False
    disclaimer: str
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)

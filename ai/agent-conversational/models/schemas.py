"""Pydantic schemas for conversational agent."""
from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field


class OnboardingStage(StrEnum):
    INITIAL = "initial"
    DOCUMENTS = "documents"
    VERIFICATION = "verification"
    FINALIZATION = "finalization"


class ChatRequest(BaseModel):
    tenant_id: str
    session_id: str
    message: str
    stage: OnboardingStage = OnboardingStage.INITIAL
    context: dict[str, Any] = Field(default_factory=dict)


class ChatResponse(BaseModel):
    response: str
    suggested_next_steps: list[str] = Field(default_factory=list)
    require_human_handoff: bool = False
    gateway_metadata: dict[str, Any] = Field(default_factory=dict)

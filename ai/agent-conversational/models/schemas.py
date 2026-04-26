from __future__ import annotations
from enum import StrEnum
from pydantic import BaseModel, Field


class OnboardingStage(StrEnum):
    WELCOME = "welcome"
    DOCUMENT_COLLECTION = "document_collection"
    IDENTITY_VERIFICATION = "identity_verification"
    RISK_REVIEW = "risk_review"
    ACCOUNT_OPENING = "account_opening"
    COMPLETED = "completed"


class Message(BaseModel):
    role: str   # "user" | "assistant"
    content: str


class ChatRequest(BaseModel):
    session_id: str
    tenant_id: str
    application_id: str
    stage: OnboardingStage = OnboardingStage.WELCOME
    user_message: str
    history: list[Message] = Field(default_factory=list)


class ChatResponse(BaseModel):
    session_id: str
    assistant_message: str
    updated_history: list[Message]
    needs_escalation: bool
    suggested_action: str | None = None  # e.g. "upload_document", "call_manager"
    model_used: str

from __future__ import annotations
from datetime import datetime, timezone
from enum import StrEnum
from typing import Any
from pydantic import BaseModel, Field


class EventType(StrEnum):
    APPLICATION_CREATED   = "application.created"
    APPLICATION_UPDATED   = "application.updated"
    DOCUMENT_UPLOADED     = "document.uploaded"
    DECISION_MADE         = "decision.made"
    ACCOUNT_OPENED        = "account.opened"
    IDENTITY_VERIFIED     = "identity.verified"
    RISK_SCORED           = "risk.scored"
    TENANT_CONFIG_CHANGED = "tenant_config.changed"


class AuditEvent(BaseModel):
    tenant_id:     str
    event_type:    EventType
    actor_id:      str
    actor_role:    str
    resource_type: str
    resource_id:   str
    payload:       dict[str, Any] = Field(default_factory=dict)
    occurred_at:   datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

"""Pydantic models that mirror the Go SDK in ``packages/audit-sdk``."""

from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class ActorType(StrEnum):
    """Who triggered the audited event. Matches the Go ``ActorType``."""

    USER = "user"
    SYSTEM = "system"
    AI_AGENT = "ai_agent"


class RecordEventRequest(BaseModel):
    """Request body for ``POST /v1/events``.

    ``payload`` is any JSON-serialisable mapping. Hash chain fields are
    populated server-side and must not be set here.
    """

    model_config = ConfigDict(use_enum_values=True, extra="forbid")

    tenant_id: str
    entity_type: str
    entity_id: str
    event_type: str
    actor_id: str
    actor_type: ActorType
    payload: dict[str, Any] = Field(default_factory=dict)

    def validate_required(self) -> None:
        """Cheap pre-flight validation. Pydantic already validates types,
        but we want a clear domain error for empty strings.
        """
        for name in (
            "tenant_id",
            "entity_type",
            "entity_id",
            "event_type",
            "actor_id",
        ):
            if not getattr(self, name):
                raise ValueError(f"audit: {name} is required")


class AuditEvent(BaseModel):
    """Response shape from ``POST`` and items in ``GET`` responses.

    Mirrors ``services/audit-service/internal/domain.AuditEvent``.
    """

    model_config = ConfigDict(extra="ignore")

    id: str
    tenant_id: str
    entity_type: str
    entity_id: str
    event_type: str
    actor_id: str
    actor_type: ActorType
    payload: dict[str, Any] = Field(default_factory=dict)
    previous_hash: str = ""
    hash: str = ""
    created_at: datetime


class QueryOptions(BaseModel):
    """Filters for ``GET /v1/events``.

    ``tenant_id`` is required by the server. ``limit`` is clamped 1..1000
    server-side.
    """

    model_config = ConfigDict(extra="forbid")

    tenant_id: str
    entity_type: str | None = None
    entity_id: str | None = None
    limit: int | None = None

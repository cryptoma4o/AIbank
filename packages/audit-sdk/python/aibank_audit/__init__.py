"""Tiny async client for the AIbank platform ``audit-service`` HTTP API.

The Go counterpart in :mod:`packages/audit-sdk` is the source of truth for
the wire shape; types here mirror it 1:1. Keep them in sync when the
contract evolves (see ``packages/openapi/audit-service.yaml``).
"""

from .client import AsyncAuditClient, AuditAPIError
from .types import (
    ActorType,
    AuditEvent,
    QueryOptions,
    RecordEventRequest,
)

__all__ = [
    "ActorType",
    "AsyncAuditClient",
    "AuditAPIError",
    "AuditEvent",
    "QueryOptions",
    "RecordEventRequest",
]

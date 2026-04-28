"""OTEL + audit-sdk wiring for agent-risk-scoring.

See ``ai/_shared/instrumentation.py`` for the canonical reference.
"""

from __future__ import annotations

import logging
import os
from typing import Any

log = logging.getLogger(__name__)

SERVICE_NAME = "agent-risk-scoring"
SERVICE_VERSION = "0.1.0"
EVENT_TYPE = "agent.risk_scoring.invoked"
ENTITY_TYPE = "risk_assessment"


def init_otel(app: Any) -> bool:
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if not endpoint:
        return False
    try:
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
            OTLPSpanExporter,
        )
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
        from opentelemetry.sdk.resources import Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
    except ImportError as exc:
        log.warning("OTEL deps missing for %s: %s", SERVICE_NAME, exc)
        return False
    resource = Resource.create(
        {
            "service.name": SERVICE_NAME,
            "service.version": os.getenv("OTEL_SERVICE_VERSION", SERVICE_VERSION),
        }
    )
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(provider)
    FastAPIInstrumentor.instrument_app(app)
    log.info("OTEL enabled for %s → %s", SERVICE_NAME, endpoint)
    return True


_audit_client: Any | None = None


def get_audit_client() -> Any | None:
    global _audit_client
    if _audit_client is not None:
        return _audit_client
    base_url = os.getenv("AUDIT_SERVICE_URL", "").strip()
    if not base_url:
        return None
    try:
        from aibank_audit import AsyncAuditClient
    except ImportError:
        log.warning("aibank_audit not installed; %s audit disabled", SERVICE_NAME)
        return None
    _audit_client = AsyncAuditClient(
        base_url=base_url,
        api_key=os.getenv("AUDIT_SERVICE_API_KEY") or None,
    )
    return _audit_client


def reset_audit_client() -> None:
    global _audit_client
    _audit_client = None


async def emit_invocation(
    *,
    tenant_id: str,
    entity_id: str,
    actor_id: str = "system",
    payload: dict[str, Any] | None = None,
) -> None:
    """Best-effort: publish ``agent.risk_scoring.invoked``."""
    client = get_audit_client()
    if client is None:
        return
    try:
        from aibank_audit import ActorType, RecordEventRequest

        await client.append(
            RecordEventRequest(
                tenant_id=tenant_id,
                entity_type=ENTITY_TYPE,
                entity_id=entity_id,
                event_type=EVENT_TYPE,
                actor_id=actor_id or "system",
                actor_type=ActorType.AI_AGENT,
                payload=payload or {},
            )
        )
    except Exception as exc:  # pragma: no cover - defensive
        log.warning("audit emit failed (%s): %s", EVENT_TYPE, exc)


__all__ = [
    "EVENT_TYPE",
    "ENTITY_TYPE",
    "emit_invocation",
    "get_audit_client",
    "init_otel",
    "reset_audit_client",
]

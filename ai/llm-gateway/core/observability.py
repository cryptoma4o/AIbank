"""OpenTelemetry + audit-sdk wiring for llm-gateway.

Soft dependencies: every import that pulls in OTEL or aibank-audit is guarded
so the service stays runnable (and tests stay green) when those extras are not
installed. Both subsystems are activated by environment variables:

- ``OTEL_EXPORTER_OTLP_ENDPOINT`` — enables FastAPI auto-instrumentation.
- ``OTEL_SERVICE_VERSION`` — overrides ``service.version`` resource attr.
- ``AUDIT_SERVICE_URL`` — enables audit publishing (`llm.completion`).
- ``AUDIT_SERVICE_API_KEY`` — optional bearer token.

See ``ai/_shared/instrumentation.py`` for the canonical reference / event-type
table, and ADR-0010 for the compliance rationale (every LLM completion produces
a hash-chained audit event).
"""

from __future__ import annotations

import logging
import os
from typing import Any

log = logging.getLogger(__name__)


# ── OpenTelemetry ─────────────────────────────────────────────────────────────


def init_otel(app: Any, service_name: str = "llm-gateway", version: str = "0.2.0") -> bool:
    """Set up OTLP/HTTP trace export + FastAPI middleware. Idempotent on failure."""
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
        log.warning("OTEL deps missing for %s: %s", service_name, exc)
        return False

    resource = Resource.create(
        {
            "service.name": service_name,
            "service.version": os.getenv("OTEL_SERVICE_VERSION", version),
        }
    )
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(provider)
    FastAPIInstrumentor.instrument_app(app)
    log.info("OTEL FastAPI instrumentation enabled for %s → %s", service_name, endpoint)
    return True


# ── audit-sdk ─────────────────────────────────────────────────────────────────


_audit_client: Any | None = None  # AsyncAuditClient | None — lazy & soft.


def get_audit_client() -> Any | None:
    """Return a process-wide ``AsyncAuditClient`` or ``None`` if disabled.

    Disabled cases (return ``None``):
      - ``AUDIT_SERVICE_URL`` env var is empty/unset;
      - the ``aibank_audit`` package is not installed.
    """
    global _audit_client
    if _audit_client is not None:
        return _audit_client
    base_url = os.getenv("AUDIT_SERVICE_URL", "").strip()
    if not base_url:
        return None
    try:
        from aibank_audit import AsyncAuditClient
    except ImportError:
        log.warning("aibank_audit not installed; audit emission disabled")
        return None
    _audit_client = AsyncAuditClient(
        base_url=base_url,
        api_key=os.getenv("AUDIT_SERVICE_API_KEY") or None,
    )
    log.info("audit-sdk client initialised → %s", base_url)
    return _audit_client


def reset_audit_client() -> None:
    """Test helper: drop the cached client between tests."""
    global _audit_client
    _audit_client = None


async def emit_llm_completion(
    *,
    tenant_id: str,
    completion_id: str,
    actor_id: str,
    model: str,
    role: str,
    prompt_tokens: int,
    completion_tokens: int,
    backend: str,
    used_fallback: bool,
) -> None:
    """Best-effort: publish an ``llm.completion`` audit event. Never raises."""
    client = get_audit_client()
    if client is None:
        return
    try:
        from aibank_audit import ActorType, RecordEventRequest

        await client.append(
            RecordEventRequest(
                tenant_id=tenant_id,
                entity_type="llm_completion",
                entity_id=completion_id,
                event_type="llm.completion",
                actor_id=actor_id or "system",
                actor_type=ActorType.AI_AGENT,
                payload={
                    "model": model,
                    "role": role,
                    "tokens_in": prompt_tokens,
                    "tokens_out": completion_tokens,
                    "backend": backend,
                    "used_fallback": used_fallback,
                },
            )
        )
    except Exception as exc:  # pragma: no cover - defensive
        log.warning("audit emit failed (llm.completion): %s", exc)


__all__ = [
    "emit_llm_completion",
    "get_audit_client",
    "init_otel",
    "reset_audit_client",
]

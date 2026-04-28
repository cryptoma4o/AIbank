"""Shared OpenTelemetry + audit-sdk wiring helpers for AIbank AI services.

This module is the **canonical reference**: each Python AI service under ``ai/``
has its own virtualenv and does NOT import from this file at runtime (cross-
package imports between sibling services are fragile in mono-repos with
per-service venvs). Each service inlines an equivalent ``init_otel()`` /
``audit_emit()`` snippet in its ``main.py`` (see ``ai/llm-gateway/main.py`` for
the canonical example).

Why duplication is acceptable:

- Snippets are <40 lines and stable.
- Hard-failing on missing OTEL/audit deps would break tests; each snippet
  guards optional imports with ``try/except ImportError`` so the service stays
  green when only the core ``fastapi`` deps are installed.
- A single source of truth here lets reviewers grep for the pattern.

Environment variables consumed by every service:

- ``OTEL_EXPORTER_OTLP_ENDPOINT`` — when set, FastAPI auto-instrumentation
  is enabled and spans are sent over OTLP/HTTP. When empty, OTEL is a no-op.
- ``OTEL_SERVICE_VERSION`` — overrides the default ``service.version``
  resource attribute.
- ``AUDIT_SERVICE_URL`` — when set, ``aibank_audit.AsyncAuditClient`` is
  initialised; agent endpoints publish events on success. When empty,
  audit emission is a no-op (logged at DEBUG).
- ``AUDIT_SERVICE_API_KEY`` — optional bearer token for audit-service.

Audit event types defined across services (entity_type → event_type):

==============================  ==============================
Service                         event_type
==============================  ==============================
llm-gateway                     ``llm.completion``
agent-document-intake           ``agent.document_intake.invoked``
agent-reconciliation            ``agent.reconciliation.invoked``
agent-ubo-tracing               ``agent.ubo_tracing.invoked``
agent-conversational            ``agent.conversational.invoked``
agent-risk-scoring              ``agent.risk_scoring.invoked``
agent-compliance-assistant      ``agent.compliance.invoked``
rag-service                     ``rag.search``
==============================  ==============================

See ``docs/security-architecture.md`` § 8 (logging) and ADR-0010 (audit chain)
for the compliance rationale.
"""

from __future__ import annotations

import logging
import os
from typing import Any

log = logging.getLogger(__name__)


def init_otel(app: Any, service_name: str, version: str = "0.1.0") -> bool:
    """Initialise OpenTelemetry FastAPI auto-instrumentation.

    Returns ``True`` when instrumentation was activated, ``False`` otherwise
    (no endpoint configured or OTEL libs missing). Never raises.

    Inline equivalent for each service::

        if os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
            try:
                from opentelemetry import trace
                from opentelemetry.sdk.resources import Resource
                from opentelemetry.sdk.trace import TracerProvider
                from opentelemetry.sdk.trace.export import BatchSpanProcessor
                from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
                from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

                resource = Resource.create({
                    "service.name": "<service-name>",
                    "service.version": os.getenv("OTEL_SERVICE_VERSION", "<version>"),
                })
                provider = TracerProvider(resource=resource)
                provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
                trace.set_tracer_provider(provider)
                FastAPIInstrumentor.instrument_app(app)
            except ImportError:
                pass  # OTEL extras not installed; treat as disabled.
    """
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


__all__ = ["init_otel"]

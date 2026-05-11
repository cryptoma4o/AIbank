"""E2E проверка audit-service (append-only event log).

Создаём событие → читаем через GET и убеждаемся, что оно есть.
Audit — критичная часть compliance (115-ФЗ), append-only без удалений.
"""

from __future__ import annotations

import uuid

import pytest

from .helpers import http_request, url


@pytest.mark.audit
def test_audit_write_then_read(base_url: str, tenant: str) -> None:
    entity_id = f"e2e-{uuid.uuid4().hex[:12]}"
    payload = {
        "tenant_id": tenant,
        "entity_type": "test",
        "entity_id": entity_id,
        "event_type": "e2e.audit_smoke",
        "actor_id": "e2e-test",
        "actor_type": "system",
        "payload": {"source": "tests/e2e/api/test_audit_trail.py"},
    }
    write_resp = http_request("POST", url(base_url, "audit", "/v1/events"), json_body=payload)
    assert write_resp.get("_status") == 201, f"expected 201, got {write_resp}"

    list_resp = http_request(
        "GET",
        url(base_url, "audit", f"/v1/events?tenant_id={tenant}&entity_id={entity_id}&limit=10"),
    )
    assert list_resp.get("_status") == 200
    # audit-service возвращает {"count": N, "items": [...]} (см. /opt/aibank/services/audit-service)
    events = list_resp.get("items") or list_resp.get("events") or list_resp.get("data") or []
    assert events, f"audit list empty for entity_id={entity_id}: {list_resp}"

    matching = [e for e in events if e.get("entity_id") == entity_id]
    assert matching, f"event with entity_id={entity_id} not found in {events}"
    assert matching[0].get("event_type") == "e2e.audit_smoke"


@pytest.mark.audit
def test_audit_health(base_url: str) -> None:
    resp = http_request("GET", url(base_url, "audit", "/health"))
    assert resp.get("_status") == 200

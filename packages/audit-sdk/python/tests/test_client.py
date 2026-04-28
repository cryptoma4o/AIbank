"""Tests for :mod:`aibank_audit.client`.

Uses ``respx`` to mock the audit-service so we don't need a live HTTP
server and can deterministically simulate retries.
"""

from __future__ import annotations

import json
from datetime import datetime, timezone

import httpx
import pytest
import respx

from aibank_audit import (
    ActorType,
    AsyncAuditClient,
    AuditAPIError,
    AuditEvent,
    QueryOptions,
    RecordEventRequest,
)


BASE = "http://audit.local"


def _ev_response(req: RecordEventRequest) -> dict:
    return {
        "id": "evt_123",
        "tenant_id": req.tenant_id,
        "entity_type": req.entity_type,
        "entity_id": req.entity_id,
        "event_type": req.event_type,
        "actor_id": req.actor_id,
        "actor_type": req.actor_type,
        "payload": req.payload,
        "previous_hash": "",
        "hash": "deadbeef",
        "created_at": datetime.now(timezone.utc).isoformat(),
    }


def _make_request() -> RecordEventRequest:
    return RecordEventRequest(
        tenant_id="bank_alpha",
        entity_type="application",
        entity_id="app_42",
        event_type="application.created",
        actor_id="user_7",
        actor_type=ActorType.USER,
        payload={"channel": "web"},
    )


@respx.mock
async def test_append_happy_path() -> None:
    req = _make_request()
    route = respx.post(f"{BASE}/v1/events").mock(
        return_value=httpx.Response(201, json=_ev_response(req))
    )
    async with AsyncAuditClient(base_url=BASE) as c:
        ev = await c.append(req)
    assert isinstance(ev, AuditEvent)
    assert ev.id == "evt_123"
    assert ev.tenant_id == "bank_alpha"
    assert route.called


@respx.mock
async def test_append_validation_failure() -> None:
    bad = _make_request()
    bad.tenant_id = ""
    async with AsyncAuditClient(base_url=BASE) as c:
        with pytest.raises(ValueError, match="tenant_id"):
            await c.append(bad)


@respx.mock
async def test_append_4xx_raises_api_error_no_retry() -> None:
    route = respx.post(f"{BASE}/v1/events").mock(
        return_value=httpx.Response(
            400,
            json={"error": {"code": "validation_failed", "message": "bad"}},
        )
    )
    async with AsyncAuditClient(base_url=BASE, max_retries=3) as c:
        with pytest.raises(AuditAPIError) as info:
            await c.append(_make_request())
    assert info.value.status_code == 400
    assert info.value.code == "validation_failed"
    assert route.call_count == 1


@respx.mock
async def test_append_retries_on_503_then_succeeds() -> None:
    req = _make_request()
    route = respx.post(f"{BASE}/v1/events").mock(
        side_effect=[
            httpx.Response(503),
            httpx.Response(503),
            httpx.Response(201, json=_ev_response(req)),
        ]
    )
    async with AsyncAuditClient(base_url=BASE, max_retries=3) as c:
        ev = await c.append(req)
    assert ev.id == "evt_123"
    assert route.call_count == 3


@respx.mock
async def test_append_retry_budget_exhausted() -> None:
    route = respx.post(f"{BASE}/v1/events").mock(
        return_value=httpx.Response(503)
    )
    async with AsyncAuditClient(base_url=BASE, max_retries=3) as c:
        with pytest.raises(AuditAPIError) as info:
            await c.append(_make_request())
    assert info.value.status_code == 503
    assert route.call_count == 3


@respx.mock
async def test_list_with_filters() -> None:
    captured: dict[str, str] = {}

    def _handler(request: httpx.Request) -> httpx.Response:
        captured.update(request.url.params)
        return httpx.Response(
            200,
            json={
                "items": [
                    {
                        "id": "evt_1",
                        "tenant_id": "bank_alpha",
                        "entity_type": "application",
                        "entity_id": "app_42",
                        "event_type": "application.created",
                        "actor_id": "u",
                        "actor_type": "user",
                        "payload": {},
                        "previous_hash": "",
                        "hash": "h1",
                        "created_at": "2026-04-26T00:00:00Z",
                    }
                ],
                "count": 1,
            },
        )

    respx.get(f"{BASE}/v1/events").mock(side_effect=_handler)

    async with AsyncAuditClient(base_url=BASE) as c:
        items = await c.list(
            QueryOptions(
                tenant_id="bank_alpha",
                entity_type="application",
                entity_id="app_42",
                limit=50,
            )
        )

    assert len(items) == 1
    assert items[0].id == "evt_1"
    assert captured == {
        "tenant_id": "bank_alpha",
        "entity_type": "application",
        "entity_id": "app_42",
        "limit": "50",
    }


@respx.mock
async def test_list_requires_tenant() -> None:
    async with AsyncAuditClient(base_url=BASE) as c:
        with pytest.raises(ValueError, match="tenant_id"):
            # type: ignore[arg-type]  # we intentionally bypass model validation here
            await c.list(QueryOptions.model_construct(tenant_id=""))


@respx.mock
async def test_authorization_header_when_api_key_set() -> None:
    seen: dict[str, str] = {}

    def _handler(request: httpx.Request) -> httpx.Response:
        seen["auth"] = request.headers.get("authorization", "")
        return httpx.Response(201, json=_ev_response(_make_request()))

    respx.post(f"{BASE}/v1/events").mock(side_effect=_handler)

    async with AsyncAuditClient(base_url=BASE, api_key="s3cret") as c:
        await c.append(_make_request())

    assert seen["auth"] == "Bearer s3cret"


def test_request_model_dump_round_trip() -> None:
    req = _make_request()
    body = json.loads(req.model_dump_json())
    assert body["actor_type"] == "user"
    assert body["payload"] == {"channel": "web"}

"""Audit-emission test: a successful /v1/trace publishes one event."""
from __future__ import annotations

import json
from typing import Any
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import observability as obs_mod
from agent import tracer as tracer_mod


class _FakeGateway:
    def __init__(self, payload: dict[str, Any]) -> None:
        self.payload = payload

    async def chat(self, role: str, messages: list[dict[str, str]],
                   tenant_id: str, max_tokens: int = 1024,
                   temperature: float = 0.1) -> dict[str, Any]:
        return {
            "choices": [{"message": {"content": json.dumps(self.payload, ensure_ascii=False)}}],
            "aibank_gateway": {
                "role": role, "resolved_model": "mock-fast", "backend": "mock",
                "used_fallback": False, "tenant_id": tenant_id, "latency_ms": 0.5,
            },
        }


@pytest.fixture
def client_with_audit(monkeypatch: pytest.MonkeyPatch):
    fake_gw = _FakeGateway({
        "nodes": [
            {"id": "7700000001", "type": "legal_entity", "name": "ООО"},
            {"id": "p1", "type": "person", "name": "Иванов"},
        ],
        "edges": [{"from": "p1", "to": "7700000001", "share_percent": 100.0}],
        "ubos": [{
            "person_id": "p1", "name": "Иванов",
            "effective_share_percent": 100.0,
            "control_basis": "ownership", "paths": [["p1", "7700000001"]],
        }],
        "confidence": 0.9, "unresolved_branches": [],
    })
    monkeypatch.setattr(tracer_mod, "GatewayClient", lambda *a, **kw: fake_gw)

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def test_trace_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    body = {
        "tenant_id": "bank-alpha",
        "application_id": "app-99",
        "root_inn": "7700000001",
        "ownership_extracts": [{
            "legal_entity_inn": "7700000001",
            "legal_entity_name": "ООО",
            "owners": [{"id": "p1", "type": "person", "name": "Иванов",
                        "share_percent": 100.0}],
        }],
    }
    resp = client.post(
        "/v1/trace", json=body, headers={"X-Actor-ID": "pipeline-1"}
    )
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "agent.ubo_tracing.invoked"
    assert req.entity_type == "ubo_trace"
    assert req.entity_id == "app-99"
    assert req.payload["ubo_count"] == 1

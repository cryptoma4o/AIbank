"""Audit-emission test: a successful /v1/chat publishes one event."""
from __future__ import annotations

import json
from typing import Any
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import chat as chat_mod
from agent import observability as obs_mod


class _FakeGateway:
    def __init__(self, payload: dict[str, Any]) -> None:
        self.payload = payload

    async def chat(self, role: str, messages: list[dict[str, str]],
                   tenant_id: str, max_tokens: int = 400,
                   temperature: float = 0.4) -> dict[str, Any]:
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
        "response": "Здравствуйте.",
        "suggested_next_steps": [],
        "require_human_handoff": False,
    })
    monkeypatch.setattr(chat_mod, "GatewayClient", lambda *a, **kw: fake_gw)

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def test_chat_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    body = {
        "tenant_id": "bank-alpha",
        "session_id": "sess-1",
        "message": "Здравствуйте",
        "stage": "initial",
    }
    resp = client.post("/v1/chat", json=body, headers={"X-Actor-ID": "ui-frontend"})
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "agent.conversational.invoked"
    assert req.entity_type == "chat_session"
    assert req.entity_id == "sess-1"
    assert req.actor_id == "ui-frontend"
    assert req.payload["stage"] == "initial"

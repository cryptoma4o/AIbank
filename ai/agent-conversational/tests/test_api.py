"""HTTP-level tests for the conversational FastAPI app."""
from __future__ import annotations

import json
from typing import Any

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import chat as chat_mod


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
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    fake = _FakeGateway({
        "response": "Для ИП нужен паспорт и ИНН.",
        "suggested_next_steps": ["Подготовьте паспорт"],
        "require_human_handoff": False,
    })
    monkeypatch.setattr(chat_mod, "GatewayClient", lambda *a, **kw: fake)
    return TestClient(main_mod.app)


def test_chat_endpoint_returns_response(client: TestClient) -> None:
    body = {
        "tenant_id": "bank-alpha",
        "session_id": "s1",
        "message": "Какие документы нужны для ИП?",
        "stage": "initial",
        "context": {},
    }
    resp = client.post("/v1/chat", json=body)
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert data["require_human_handoff"] is False
    assert "паспорт" in data["response"].lower()
    assert data["suggested_next_steps"]
    assert data["gateway_metadata"]["resolved_model"] == "mock-fast"


def test_chat_missing_tenant_id_returns_400(client: TestClient) -> None:
    body = {
        "tenant_id": "",
        "session_id": "s1",
        "message": "Hi",
        "stage": "initial",
    }
    resp = client.post("/v1/chat", json=body)
    assert resp.status_code == 400


def test_chat_complaint_short_circuits(client: TestClient) -> None:
    body = {
        "tenant_id": "bank-alpha",
        "session_id": "s2",
        "message": "У меня жалоба на банк!",
        "stage": "verification",
    }
    resp = client.post("/v1/chat", json=body)
    assert resp.status_code == 200
    data = resp.json()
    assert data["require_human_handoff"] is True


def test_healthz(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"

"""HTTP-level tests for the reconciliation FastAPI app."""
from __future__ import annotations

import json
from typing import Any

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import reconciler as reconciler_mod


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
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    fake = _FakeGateway({
        "matches": False,
        "discrepancies": [{
            "field": "director", "extracted_value": "Иванов",
            "registry_value": "Петров", "severity": "high",
        }],
        "questions_for_client": ["Уточните руководителя."],
        "requires_review": True,
    })
    monkeypatch.setattr(reconciler_mod, "GatewayClient", lambda *a, **kw: fake)
    return TestClient(main_mod.app)


def test_reconcile_endpoint_returns_structured_result(client: TestClient) -> None:
    body = {
        "tenant_id": "bank-alpha",
        "application_id": "app-99",
        "extracted_data": {"director": "Иванов"},
        "egrul_data": {"director": "Петров"},
    }
    resp = client.post("/v1/reconcile", json=body)
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert data["matches"] is False
    assert data["requires_review"] is True
    assert data["discrepancies"]
    assert data["questions_for_client"]
    assert data["gateway_metadata"]["resolved_model"] == "mock-fast"


def test_reconcile_missing_tenant_id_returns_400(client: TestClient) -> None:
    body = {
        "tenant_id": "",
        "application_id": "app-99",
        "extracted_data": {"a": 1},
        "egrul_data": {"a": 1},
    }
    resp = client.post("/v1/reconcile", json=body)
    assert resp.status_code == 400


def test_healthz(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"

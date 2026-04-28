"""HTTP-level tests for the UBO tracing FastAPI app."""
from __future__ import annotations

import json
from typing import Any

import pytest
from fastapi.testclient import TestClient

import main as main_mod
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
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    fake = _FakeGateway({
        "nodes": [
            {"id": "7700000001", "type": "legal_entity", "name": "ООО"},
            {"id": "p1", "type": "person", "name": "Иванов И.И."},
        ],
        "edges": [{"from": "p1", "to": "7700000001", "share_percent": 100.0}],
        "ubos": [{
            "person_id": "p1", "name": "Иванов И.И.",
            "effective_share_percent": 100.0,
            "control_basis": "ownership", "paths": [["p1", "7700000001"]],
        }],
        "confidence": 0.9, "unresolved_branches": [],
    })
    monkeypatch.setattr(tracer_mod, "GatewayClient", lambda *a, **kw: fake)
    return TestClient(main_mod.app)


def test_trace_endpoint_returns_ubos(client: TestClient) -> None:
    body = {
        "tenant_id": "bank-alpha",
        "application_id": "app-99",
        "root_inn": "7700000001",
        "ownership_extracts": [{
            "legal_entity_inn": "7700000001",
            "legal_entity_name": "ООО",
            "owners": [{"id": "p1", "type": "person", "name": "Иванов И.И.",
                        "share_percent": 100.0}],
        }],
    }
    resp = client.post("/v1/trace", json=body)
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert len(data["ubos"]) == 1
    assert data["ubos"][0]["person_id"] == "p1"
    assert data["confidence"] == pytest.approx(0.9)
    assert data["gateway_metadata"]["resolved_model"] == "mock-fast"


def test_trace_missing_tenant_id_returns_400(client: TestClient) -> None:
    body = {
        "tenant_id": "",
        "application_id": "app-99",
        "root_inn": "7700000001",
        "ownership_extracts": [],
    }
    resp = client.post("/v1/trace", json=body)
    assert resp.status_code == 400


def test_healthz(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"

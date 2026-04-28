"""HTTP-level tests for agent-risk-scoring FastAPI app."""
from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import scorer as scorer_mod

from tests.conftest import FakeGateway


@pytest.fixture
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    fake = FakeGateway(content="Заключение от LLM: риск низкий, рекомендовано одобрение.")
    monkeypatch.setattr(scorer_mod, "GatewayClient", lambda *a, **kw: fake)
    return TestClient(main_mod.app)


def test_healthz(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_score_endpoint_returns_full_payload(client: TestClient) -> None:
    body = {
        "application_id": "app-1",
        "tenant_id": "bank-alpha",
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "okved": "62.01",
        "company_age_years": 5,
    }
    resp = client.post("/v1/score", json=body)
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert data["application_id"] == "app-1"
    assert data["recommendation"] == "approve"
    assert data["explanation_source"] == "llm"
    assert data["model_used"] == "mock-fast"
    assert "Заключение от LLM" in data["explanation"]
    assert data["gateway_metadata"]["resolved_model"] == "mock-fast"


def test_score_blocks_on_sanctions(client: TestClient) -> None:
    body = {
        "application_id": "app-2",
        "tenant_id": "bank-alpha",
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "okved": "62.01",
        "company_age_years": 10,
        "facts": {"sanctions_match": True},
    }
    resp = client.post("/v1/score", json=body)
    assert resp.status_code == 200
    data = resp.json()
    assert data["blocked"] is True
    assert data["recommendation"] == "reject"
    assert "sanctions_match" in data["flags"]


def test_score_missing_tenant_returns_400(client: TestClient) -> None:
    body = {
        "application_id": "app-x",
        "tenant_id": "",
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "okved": "62.01",
        "company_age_years": 5,
    }
    resp = client.post("/v1/score", json=body)
    assert resp.status_code == 400

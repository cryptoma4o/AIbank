"""HTTP-level tests for agent-document-intake FastAPI app."""
from __future__ import annotations

import base64

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import extractor as extractor_mod

from tests.conftest import FakeGateway


def _b64(s: str) -> str:
    return base64.b64encode(s.encode("utf-8")).decode("ascii")


@pytest.fixture
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    fake = FakeGateway(content={"inn": "7707083893", "ogrn": "1027700132195"})
    monkeypatch.setattr(extractor_mod, "GatewayClient", lambda *a, **kw: fake)
    return TestClient(main_mod.app)


def test_healthz(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_extract_endpoint_returns_parsed_data(client: TestClient) -> None:
    body = {
        "document_id": "doc-1",
        "tenant_id": "bank-alpha",
        "document_type": "egrul",
        "content_base64": _b64("ИНН 7707083893 ОГРН 1027700132195"),
    }
    resp = client.post("/v1/extract", json=body)
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert data["document_id"] == "doc-1"
    assert data["model_used"] == "mock-fast"
    field_map = {f["name"]: f for f in data["fields"]}
    assert field_map["inn"]["value"] == "7707083893"
    assert field_map["ogrn"]["value"] == "1027700132195"
    assert all(f["confidence"] >= 0.8 for f in data["fields"])
    assert data["gateway_metadata"]["resolved_model"] == "mock-fast"


def test_extract_missing_tenant_returns_400(client: TestClient) -> None:
    body = {
        "document_id": "doc-x",
        "tenant_id": "",
        "document_type": "egrul",
        "content_base64": _b64("ИНН 7707083893"),
    }
    resp = client.post("/v1/extract", json=body)
    assert resp.status_code == 400

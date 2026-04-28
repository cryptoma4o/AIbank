"""HTTP-level tests for rag-service. Backend is monkeypatched to MemoryStore."""
from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from service.embedder import MockEmbedder
from service.qdrant_client import MemoryStore
from service.retriever import Retriever


@pytest.fixture
def client(monkeypatch: pytest.MonkeyPatch) -> TestClient:
    # Replace the Qdrant-backed default store with the deterministic memory store.
    monkeypatch.setattr(
        main_mod, "build_default_store", lambda: MemoryStore()
    )
    tc = TestClient(main_mod.app)
    # TestClient triggers `startup` via context manager when used with `with`,
    # but using it as a fixture also fires startup events.
    return tc


def _index_sample(tc: TestClient, tenant: str = "bank-alpha") -> None:
    body = {
        "tenant_id": tenant,
        "documents": [
            {
                "id": "115-fz-art7",
                "text": "Статья 7 115-ФЗ — идентификация клиента и бенефициарных владельцев.",
                "source": "115-ФЗ ст.7",
                "source_type": "law",
            },
            {
                "id": "375-p",
                "text": "Положение 375-П — факторы повышенного риска ОД/ФТ.",
                "source": "375-П",
                "source_type": "regulation",
            },
        ],
    }
    resp = tc.post("/v1/index", json=body)
    assert resp.status_code == 200, resp.text
    assert resp.json()["indexed"] == 2


def test_healthz_reports_backend(client: TestClient) -> None:
    resp = client.get("/healthz")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["backend"] == "memory"


def test_index_then_search(client: TestClient) -> None:
    _index_sample(client)
    resp = client.post(
        "/v1/search",
        json={
            "tenant_id": "bank-alpha",
            "query": "идентификация бенефициарных владельцев",
            "top_k": 3,
        },
    )
    assert resp.status_code == 200, resp.text
    data = resp.json()
    assert data["tenant_id"] == "bank-alpha"
    assert data["backend"] == "memory"
    assert data["hits"], "expected at least one hit"
    assert data["hits"][0]["document_id"] == "115-fz-art7"
    assert data["hits"][0]["source_type"] == "law"


def test_search_with_source_type_filter(client: TestClient) -> None:
    _index_sample(client)
    resp = client.post(
        "/v1/search",
        json={
            "tenant_id": "bank-alpha",
            "query": "риск ОД ФТ",
            "filters": {"source_type": "regulation"},
        },
    )
    assert resp.status_code == 200
    hits = resp.json()["hits"]
    assert hits and all(h["source_type"] == "regulation" for h in hits)
    assert hits[0]["document_id"] == "375-p"


def test_search_missing_tenant_returns_400(client: TestClient) -> None:
    resp = client.post(
        "/v1/search",
        json={"tenant_id": "", "query": "что-то"},
    )
    assert resp.status_code == 400


def test_search_empty_query_returns_400(client: TestClient) -> None:
    resp = client.post(
        "/v1/search",
        json={"tenant_id": "bank-alpha", "query": "   "},
    )
    assert resp.status_code == 400


def test_index_missing_tenant_returns_400(client: TestClient) -> None:
    resp = client.post(
        "/v1/index",
        json={"tenant_id": "", "documents": []},
    )
    assert resp.status_code == 400


def test_tenant_isolation_through_api(client: TestClient) -> None:
    _index_sample(client, tenant="bank-alpha")
    # bank-beta hasn't indexed anything.
    resp = client.post(
        "/v1/search",
        json={"tenant_id": "bank-beta", "query": "115-ФЗ", "top_k": 5},
    )
    assert resp.status_code == 200
    assert resp.json()["hits"] == []

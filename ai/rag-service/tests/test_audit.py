"""Audit-emission test: a successful /v1/search publishes one event."""
from __future__ import annotations

from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from service import observability as obs_mod
from service.qdrant_client import MemoryStore


@pytest.fixture
def client_with_audit(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(main_mod, "build_default_store", lambda: MemoryStore())

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def _index(tc: TestClient) -> None:
    body = {
        "tenant_id": "bank-alpha",
        "documents": [
            {
                "id": "115-fz-art7",
                "text": "Статья 7 115-ФЗ — идентификация клиента.",
                "source": "115-ФЗ ст.7",
                "source_type": "law",
            }
        ],
    }
    resp = tc.post("/v1/index", json=body)
    assert resp.status_code == 200, resp.text


def test_search_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    _index(client)
    # /v1/index is admin and does NOT emit. Reset call count.
    fake_audit.append.reset_mock()

    resp = client.post(
        "/v1/search",
        json={
            "tenant_id": "bank-alpha",
            "query": "идентификация клиента",
            "top_k": 3,
        },
        headers={"X-Actor-ID": "compliance-svc"},
    )
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "rag.search"
    assert req.entity_type == "search_query"
    # entity_id is a 16-char sha256 prefix of the query — never the raw query.
    assert len(req.entity_id) == 16
    assert "идентификация" not in req.entity_id
    assert req.actor_id == "compliance-svc"
    assert req.payload["hit_count"] >= 1

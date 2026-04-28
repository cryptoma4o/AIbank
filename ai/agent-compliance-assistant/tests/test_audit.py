"""Audit-emission test: a successful /v1/answer publishes one event."""
from __future__ import annotations

from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import answerer as answerer_mod
from agent import observability as obs_mod

from tests.conftest import FakeGateway, FakeRAGClient, SAMPLE_HIT_115FZ_ART7


@pytest.fixture
def client_with_audit(monkeypatch: pytest.MonkeyPatch):
    fake_gw = FakeGateway(
        content="Банк обязан идентифицировать клиента [115-fz-art7]."
    )
    fake_rag = FakeRAGClient(hits=[SAMPLE_HIT_115FZ_ART7])
    monkeypatch.setattr(answerer_mod, "GatewayClient", lambda *a, **kw: fake_gw)
    monkeypatch.setattr(answerer_mod, "RAGClient", lambda *a, **kw: fake_rag)

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def test_answer_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    body = {
        "tenant_id": "bank-alpha",
        "question": "Должен ли банк идентифицировать клиента?",
        "top_k": 3,
    }
    resp = client.post(
        "/v1/answer", json=body, headers={"X-Actor-ID": "support-app"}
    )
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "agent.compliance.invoked"
    assert req.entity_type == "compliance_query"
    # entity_id is a 16-char sha256 prefix of the question — never the raw question.
    assert len(req.entity_id) == 16
    assert "Должен" not in req.entity_id
    assert req.actor_id == "support-app"
    assert req.payload["citation_count"] == 1

"""Audit-emission test: a successful /v1/score publishes one event."""
from __future__ import annotations

from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import observability as obs_mod
from agent import scorer as scorer_mod

from tests.conftest import FakeGateway


@pytest.fixture
def client_with_audit(monkeypatch: pytest.MonkeyPatch):
    fake_gw = FakeGateway(content="Заключение: риск низкий.")
    monkeypatch.setattr(scorer_mod, "GatewayClient", lambda *a, **kw: fake_gw)

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def test_score_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    body = {
        "application_id": "app-1",
        "tenant_id": "bank-alpha",
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "okved": "62.01",
        "company_age_years": 5,
    }
    resp = client.post(
        "/v1/score", json=body, headers={"X-Actor-ID": "pipeline-1"}
    )
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "agent.risk_scoring.invoked"
    assert req.entity_type == "risk_assessment"
    assert req.entity_id == "app-1"
    assert req.payload["recommendation"] == "approve"

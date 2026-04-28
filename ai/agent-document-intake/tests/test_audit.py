"""Audit-emission test: a successful /v1/extract publishes one event."""
from __future__ import annotations

import base64
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

import main as main_mod
from agent import extractor as extractor_mod
from agent import observability as obs_mod

from tests.conftest import FakeGateway


def _b64(s: str) -> str:
    return base64.b64encode(s.encode("utf-8")).decode("ascii")


@pytest.fixture
def client_with_audit(monkeypatch: pytest.MonkeyPatch) -> tuple[TestClient, AsyncMock]:
    fake_gw = FakeGateway(content={"inn": "7707083893", "ogrn": "1027700132195"})
    monkeypatch.setattr(extractor_mod, "GatewayClient", lambda *a, **kw: fake_gw)

    fake_audit = AsyncMock()
    fake_audit.append = AsyncMock(return_value={"id": "evt-1"})
    obs_mod.reset_audit_client()
    obs_mod._audit_client = fake_audit
    yield TestClient(main_mod.app), fake_audit
    obs_mod.reset_audit_client()


def test_extract_publishes_audit_event(client_with_audit) -> None:
    client, fake_audit = client_with_audit
    body = {
        "document_id": "doc-1",
        "tenant_id": "bank-alpha",
        "document_type": "egrul",
        "content_base64": _b64("ИНН 7707083893 ОГРН 1027700132195"),
    }
    resp = client.post(
        "/v1/extract", json=body, headers={"X-Actor-ID": "agent-pipeline"}
    )
    assert resp.status_code == 200, resp.text

    assert fake_audit.append.await_count == 1
    req = fake_audit.append.await_args.args[0]
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "agent.document_intake.invoked"
    assert req.entity_type == "document_extraction"
    assert req.entity_id == "doc-1"
    assert req.actor_id == "agent-pipeline"
    assert req.payload["document_type"] == "egrul"


def test_extract_works_without_audit_configured(monkeypatch: pytest.MonkeyPatch) -> None:
    """When AUDIT_SERVICE_URL is unset, the endpoint still succeeds."""
    fake_gw = FakeGateway(content={"inn": "7707083893"})
    monkeypatch.setattr(extractor_mod, "GatewayClient", lambda *a, **kw: fake_gw)
    monkeypatch.delenv("AUDIT_SERVICE_URL", raising=False)
    obs_mod.reset_audit_client()

    client = TestClient(main_mod.app)
    resp = client.post(
        "/v1/extract",
        json={
            "document_id": "doc-2",
            "tenant_id": "bank-alpha",
            "document_type": "egrul",
            "content_base64": _b64("ИНН 7707083893"),
        },
    )
    assert resp.status_code == 200
    assert obs_mod.get_audit_client() is None

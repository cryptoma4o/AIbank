"""Audit-emission tests: every successful chat completion publishes an event.

These tests do NOT require a running audit-service — the ``aibank_audit``
client is monkeypatched with an ``AsyncMock`` factory.
"""
from __future__ import annotations

from typing import Any
from unittest.mock import AsyncMock

import pytest
import yaml
from fastapi.testclient import TestClient


class _FakeAuditClient:
    """Minimal stand-in for ``aibank_audit.AsyncAuditClient``.

    Only ``append`` is exercised by the gateway. We record every call so
    tests can assert on tenant/event_type/payload.
    """

    def __init__(self, *_, **__) -> None:
        self.append = AsyncMock(return_value={"id": "evt-1", "hash": "x"})


@pytest.fixture
def app_with_audit(routing_config_dict, tmp_path, monkeypatch):
    cfg_path = tmp_path / "routing.yaml"
    cfg_path.write_text(yaml.safe_dump(routing_config_dict), encoding="utf-8")
    monkeypatch.setenv("LLM_GATEWAY_CONFIG", str(cfg_path))
    monkeypatch.setenv("LLM_GATEWAY_FORCE_MOCK", "1")
    monkeypatch.setenv("AUDIT_SERVICE_URL", "http://audit:8081")

    from core import config as cfg_mod
    from core import observability as obs_mod
    from core import router as router_mod

    cfg_mod.reset_config_cache()
    router_mod.reset_router()
    obs_mod.reset_audit_client()
    cfg_mod.settings = cfg_mod.Settings()

    fake = _FakeAuditClient()
    # Bypass the import path: pre-populate the cached client.
    obs_mod._audit_client = fake

    if "main" in list(__import__("sys").modules):
        del __import__("sys").modules["main"]
    import main  # noqa: F401

    return main.app, fake


def test_chat_completion_publishes_audit_event(app_with_audit: tuple[Any, _FakeAuditClient]):
    app, fake = app_with_audit
    client = TestClient(app)
    resp = client.post(
        "/v1/chat/completions",
        headers={"X-Tenant-Id": "bank-alpha", "X-Actor-ID": "agent-conv-1"},
        json={"model": "role:chat", "messages": [{"role": "user", "content": "hi"}]},
    )
    assert resp.status_code == 200, resp.text

    # Exactly one audit event for one completion.
    assert fake.append.await_count == 1
    req = fake.append.await_args.args[0]
    # RecordEventRequest is a Pydantic model; access via attrs.
    assert req.tenant_id == "bank-alpha"
    assert req.event_type == "llm.completion"
    assert req.entity_type == "llm_completion"
    assert req.actor_id == "agent-conv-1"
    # actor_type may be the StrEnum value 'ai_agent' (use_enum_values=True)
    assert str(req.actor_type) == "ai_agent" or req.actor_type == "ai_agent"
    assert req.payload["role"] == "chat"
    assert req.payload["model"] == "qwen-test"


def test_audit_disabled_when_env_unset(routing_config_dict, tmp_path, monkeypatch):
    cfg_path = tmp_path / "routing.yaml"
    cfg_path.write_text(yaml.safe_dump(routing_config_dict), encoding="utf-8")
    monkeypatch.setenv("LLM_GATEWAY_CONFIG", str(cfg_path))
    monkeypatch.setenv("LLM_GATEWAY_FORCE_MOCK", "1")
    monkeypatch.delenv("AUDIT_SERVICE_URL", raising=False)

    from core import config as cfg_mod
    from core import observability as obs_mod
    from core import router as router_mod

    cfg_mod.reset_config_cache()
    router_mod.reset_router()
    obs_mod.reset_audit_client()
    cfg_mod.settings = cfg_mod.Settings()

    if "main" in list(__import__("sys").modules):
        del __import__("sys").modules["main"]
    import main  # noqa: F401

    client = TestClient(main.app)
    resp = client.post(
        "/v1/chat/completions",
        headers={"X-Tenant-Id": "bank-alpha"},
        json={"model": "role:chat", "messages": [{"role": "user", "content": "hi"}]},
    )
    # Disabled audit ≠ failed request.
    assert resp.status_code == 200
    assert obs_mod.get_audit_client() is None

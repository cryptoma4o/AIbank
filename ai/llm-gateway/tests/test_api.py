"""End-to-end тесты HTTP API: /v1/chat/completions, /v1/models, /v1/usage.

Используют mock backend через `force_mock=True` — реальные upstream'ы не нужны.
"""
from __future__ import annotations

from pathlib import Path

import pytest
import yaml
from fastapi.testclient import TestClient


@pytest.fixture
def app(routing_config_dict, tmp_path, monkeypatch):
    cfg_path = tmp_path / "routing.yaml"
    cfg_path.write_text(yaml.safe_dump(routing_config_dict), encoding="utf-8")
    monkeypatch.setenv("LLM_GATEWAY_CONFIG", str(cfg_path))
    monkeypatch.setenv("LLM_GATEWAY_FORCE_MOCK", "1")

    # Сбрасываем кеш settings перед импортом main.
    from core import config as cfg_mod
    from core import router as router_mod
    cfg_mod.reset_config_cache()
    router_mod.reset_router()
    # Перечитываем settings из новых env-vars.
    cfg_mod.settings = cfg_mod.Settings()

    # Импортируем main после установки env (FastAPI app создаётся при импорте).
    if "main" in list(__import__("sys").modules):
        del __import__("sys").modules["main"]
    import main  # noqa: F401

    return main.app


def test_healthz_reports_force_mock(app):
    client = TestClient(app)
    resp = client.get("/healthz")
    assert resp.status_code == 200
    body = resp.json()
    assert body["service"] == "llm-gateway"
    assert body["force_mock"] is True


def test_list_models_includes_roles_and_models(app):
    client = TestClient(app)
    resp = client.get("/v1/models")
    assert resp.status_code == 200
    ids = {item["id"] for item in resp.json()["data"]}
    # модели
    assert "gemma-test" in ids
    assert "qwen-test" in ids
    # роли с префиксом
    assert "role:vision" in ids
    assert "role:chat" in ids


def test_chat_completions_rejects_missing_tenant_header(app):
    client = TestClient(app)
    resp = client.post(
        "/v1/chat/completions",
        json={"model": "role:chat", "messages": [{"role": "user", "content": "hi"}]},
    )
    assert resp.status_code == 400
    assert "X-Tenant-Id" in resp.json()["detail"]


def test_chat_completions_role_resolves_in_mock_mode(app):
    client = TestClient(app)
    resp = client.post(
        "/v1/chat/completions",
        headers={"X-Tenant-Id": "bank-alpha"},
        json={"model": "role:vision", "messages": [{"role": "user", "content": "паспорт ИИ"}]},
    )
    assert resp.status_code == 200
    body = resp.json()
    # OpenAI-shape сохранён
    assert body["object"] == "chat.completion"
    assert body["choices"][0]["message"]["role"] == "assistant"
    # gateway-метаданные
    gw = body["aibank_gateway"]
    assert gw["tenant_id"] == "bank-alpha"
    assert gw["role"] == "vision"
    assert gw["resolved_model"] == "gemma-test"
    assert gw["backend"] == "mock"  # force_mock включён


def test_x_agent_role_header_overrides_model(app):
    client = TestClient(app)
    resp = client.post(
        "/v1/chat/completions",
        headers={"X-Tenant-Id": "bank-alpha", "X-Agent-Role": "chat"},
        json={"model": "gemma-test", "messages": [{"role": "user", "content": "hi"}]},
    )
    body = resp.json()
    assert body["aibank_gateway"]["role"] == "chat"
    # role:chat → qwen-test
    assert body["aibank_gateway"]["resolved_model"] == "qwen-test"


def test_unknown_role_returns_503(app):
    client = TestClient(app)
    resp = client.post(
        "/v1/chat/completions",
        headers={"X-Tenant-Id": "bank-alpha"},
        json={"model": "role:phantom", "messages": [{"role": "user", "content": "hi"}]},
    )
    assert resp.status_code == 503


def test_usage_accumulates_across_requests(app):
    client = TestClient(app)
    for _ in range(3):
        client.post(
            "/v1/chat/completions",
            headers={"X-Tenant-Id": "bank-alpha"},
            json={"model": "role:chat", "messages": [
                {"role": "user", "content": "Привет, я открываю счёт"}
            ]},
        )
    resp = client.get("/v1/usage", params={"tenant_id": "bank-alpha"})
    assert resp.status_code == 200
    rows = resp.json()["data"]
    assert len(rows) == 1
    assert rows[0]["model"] == "qwen-test"
    assert rows[0]["requests"] == 3
    # Стоимость >= 0 (зависит от длины ответа mock-backend).
    assert rows[0]["total_kopecks"] >= 0


def test_usage_is_isolated_per_tenant(app):
    client = TestClient(app)
    for tenant in ("bank-alpha", "bank-beta"):
        client.post(
            "/v1/chat/completions",
            headers={"X-Tenant-Id": tenant},
            json={"model": "role:chat", "messages": [{"role": "user", "content": "hi"}]},
        )
    resp = client.get("/v1/usage")
    agg = resp.json()["aggregate_by_tenant"]
    assert "bank-alpha" in agg
    assert "bank-beta" in agg
    assert agg["bank-alpha"]["requests"] == 1
    assert agg["bank-beta"]["requests"] == 1


def test_default_yaml_config_is_loadable():
    """Smoke-test: реальный configs/default-routing.yaml парсится без ошибок."""
    from core.config import load_routing_config
    here = Path(__file__).resolve().parent.parent
    cfg = load_routing_config(here / "configs" / "default-routing.yaml")
    assert "document-vision" in cfg.roles
    assert cfg.roles["document-vision"].fallback is not None

"""Тесты резолвинга ролей и моделей."""
from __future__ import annotations

import pytest

from core.config import RoutingConfig
from core.router import ModelRouter


def test_resolve_direct_model(routing_config):
    r = ModelRouter(routing_config)
    res = r.resolve("gemma-test")
    assert res is not None
    assert res.role is None
    assert res.model_name == "gemma-test"
    assert res.backend.name == "vllm-real"
    assert res.cost_per_1k_input_kop == 100


def test_resolve_role_with_fallback(routing_config):
    r = ModelRouter(routing_config)
    res = r.resolve("role:vision")
    assert res is not None
    assert res.role == "vision"
    assert res.model_name == "gemma-test"
    assert res.fallback_model == "qwen-test"


def test_resolve_role_without_fallback(routing_config):
    r = ModelRouter(routing_config)
    res = r.resolve("role:chat")
    assert res is not None
    assert res.fallback_model is None


def test_resolve_unknown_role(routing_config):
    r = ModelRouter(routing_config)
    assert r.resolve("role:nonexistent") is None


def test_resolve_unknown_model(routing_config):
    r = ModelRouter(routing_config)
    assert r.resolve("gpt-7-omega") is None


def test_force_mock_redirects_to_mock(routing_config):
    r = ModelRouter(routing_config, force_mock=True)
    res = r.resolve("role:vision")
    assert res is not None
    # Имя модели сохраняется (важно для биллинга), но backend = mock
    assert res.model_name == "gemma-test"
    assert res.backend.name == "mock"


def test_invalid_config_unknown_backend_ref():
    bad = {
        "version": 1,
        "backends": {"only-mock": {"type": "mock"}},
        "models": {"orphan": {"backend": "nonexistent"}},
        "roles": {},
    }
    with pytest.raises(ValueError, match="unknown backend"):
        RoutingConfig.model_validate(bad)


def test_invalid_config_unknown_model_in_role():
    bad = {
        "version": 1,
        "backends": {"only-mock": {"type": "mock"}},
        "models": {"m1": {"backend": "only-mock"}},
        "roles": {"r": {"model": "ghost"}},
    }
    with pytest.raises(ValueError, match="unknown model"):
        RoutingConfig.model_validate(bad)


def test_invalid_config_real_backend_without_url():
    bad = {
        "version": 1,
        "backends": {"vllm": {"type": "openai_compatible"}},
        "models": {},
        "roles": {},
    }
    with pytest.raises(ValueError, match="requires url"):
        RoutingConfig.model_validate(bad)

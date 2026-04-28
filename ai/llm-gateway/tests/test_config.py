"""Тесты для core.config — особенно для apply_env_overrides (live vLLM mode)."""
from __future__ import annotations

import pytest

from core.config import RoutingConfig, apply_env_overrides


@pytest.fixture
def base_config_dict() -> dict:
    return {
        "version": 1,
        "backends": {
            "vllm-gemma": {
                "type": "openai_compatible",
                "url": "http://default-gemma:8000",
                "timeout_s": 120,
            },
            "vllm-qwen": {
                "type": "openai_compatible",
                "url": "http://default-qwen:8000",
                "timeout_s": 120,
            },
            "vllm-tpro": {
                "type": "openai_compatible",
                "url": "http://default-tpro:8000",
                "timeout_s": 60,
            },
            "mock": {"type": "mock"},
        },
        "models": {
            "gemma": {"backend": "vllm-gemma"},
            "qwen": {"backend": "vllm-qwen"},
            "tpro": {"backend": "vllm-tpro"},
            "mock-fast": {"backend": "mock"},
        },
        "roles": {
            "any": {"model": "gemma"},
        },
    }


def test_apply_env_overrides_no_env_keeps_yaml_defaults(monkeypatch, base_config_dict):
    """Без ENV-переменных URLs остаются как в YAML."""
    monkeypatch.delenv("VLLM_GEMMA_URL", raising=False)
    monkeypatch.delenv("VLLM_QWEN_URL", raising=False)
    monkeypatch.delenv("VLLM_TPRO_URL", raising=False)

    cfg = RoutingConfig.model_validate(base_config_dict)
    overridden = apply_env_overrides(cfg)

    assert overridden.backends["vllm-gemma"].url == "http://default-gemma:8000"
    assert overridden.backends["vllm-qwen"].url == "http://default-qwen:8000"
    assert overridden.backends["vllm-tpro"].url == "http://default-tpro:8000"


def test_apply_env_overrides_short_aliases(monkeypatch, base_config_dict):
    """Короткие ENV-переменные VLLM_GEMMA_URL и т.д. применяются."""
    monkeypatch.setenv("VLLM_GEMMA_URL", "http://prod-gemma.cluster.local:8000")
    monkeypatch.setenv("VLLM_QWEN_URL", "http://prod-qwen.cluster.local:8000")
    monkeypatch.setenv("VLLM_TPRO_URL", "http://prod-tpro.cluster.local:8000")

    cfg = RoutingConfig.model_validate(base_config_dict)
    overridden = apply_env_overrides(cfg)

    assert overridden.backends["vllm-gemma"].url == "http://prod-gemma.cluster.local:8000"
    assert overridden.backends["vllm-qwen"].url == "http://prod-qwen.cluster.local:8000"
    assert overridden.backends["vllm-tpro"].url == "http://prod-tpro.cluster.local:8000"


def test_apply_env_overrides_generic_form(monkeypatch, base_config_dict):
    """Generic VLLM_BACKEND_URL_<NAME> применяется для произвольных backend имён."""
    # Добавляем кастомный backend, не покрытый short-alias таблицей.
    base_config_dict["backends"]["vllm-vikhr"] = {
        "type": "openai_compatible",
        "url": "http://default-vikhr:8000",
    }
    base_config_dict["models"]["vikhr"] = {"backend": "vllm-vikhr"}

    monkeypatch.setenv("VLLM_BACKEND_URL_VLLM_VIKHR", "http://prod-vikhr:8000")

    cfg = RoutingConfig.model_validate(base_config_dict)
    overridden = apply_env_overrides(cfg)

    assert overridden.backends["vllm-vikhr"].url == "http://prod-vikhr:8000"


def test_apply_env_overrides_does_not_touch_mock(monkeypatch, base_config_dict):
    """Mock-backend не должен получать URL из ENV (он не имеет url)."""
    monkeypatch.setenv("VLLM_BACKEND_URL_MOCK", "http://hijack:8000")

    cfg = RoutingConfig.model_validate(base_config_dict)
    overridden = apply_env_overrides(cfg)

    assert overridden.backends["mock"].url is None


def test_apply_env_overrides_preserves_timeout(monkeypatch, base_config_dict):
    """Override меняет только URL — timeout_s сохраняется."""
    monkeypatch.setenv("VLLM_TPRO_URL", "http://prod-tpro:8000")

    cfg = RoutingConfig.model_validate(base_config_dict)
    overridden = apply_env_overrides(cfg)

    assert overridden.backends["vllm-tpro"].timeout_s == 60


def test_apply_env_overrides_returns_new_instance(monkeypatch, base_config_dict):
    """apply_env_overrides не мутирует оригинальный config."""
    monkeypatch.setenv("VLLM_GEMMA_URL", "http://prod-gemma:8000")

    cfg = RoutingConfig.model_validate(base_config_dict)
    original_url = cfg.backends["vllm-gemma"].url
    _ = apply_env_overrides(cfg)

    # Оригинал не изменился
    assert cfg.backends["vllm-gemma"].url == original_url

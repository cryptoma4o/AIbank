"""Pytest fixtures: общая конфигурация роутинга для тестов."""
from __future__ import annotations

import sys
from pathlib import Path

# Добавляем корень llm-gateway в sys.path, чтобы тесты могли импортировать
# `core.*` и `routers.*` без установки пакета.
HERE = Path(__file__).resolve().parent.parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))

import pytest  # noqa: E402

from core.config import RoutingConfig  # noqa: E402
from core import config as cfg_mod  # noqa: E402
from core import observability as obs_mod  # noqa: E402
from core import router as router_mod  # noqa: E402
from core import usage as usage_mod  # noqa: E402


@pytest.fixture
def routing_config_dict() -> dict:
    return {
        "version": 1,
        "backends": {
            "vllm-real": {"type": "openai_compatible", "url": "http://upstream:8000"},
            "mock": {"type": "mock"},
        },
        "models": {
            "gemma-test": {
                "backend": "vllm-real",
                "cost_per_1k_input_kop": 100,
                "cost_per_1k_output_kop": 200,
            },
            "qwen-test": {
                "backend": "vllm-real",
                "cost_per_1k_input_kop": 80,
                "cost_per_1k_output_kop": 160,
            },
            "mock-fast": {"backend": "mock"},
        },
        "roles": {
            "vision": {"model": "gemma-test", "fallback": "qwen-test"},
            "chat": {"model": "qwen-test"},
            "mock-eval": {"model": "mock-fast"},
        },
    }


@pytest.fixture
def routing_config(routing_config_dict) -> RoutingConfig:
    return RoutingConfig.model_validate(routing_config_dict)


@pytest.fixture(autouse=True)
def _reset_singletons():
    """Сбрасываем module-level кеши до и после каждого теста."""
    cfg_mod.reset_config_cache()
    router_mod.reset_router()
    usage_mod.tracker.reset()
    obs_mod.reset_audit_client()
    yield
    cfg_mod.reset_config_cache()
    router_mod.reset_router()
    usage_mod.tracker.reset()
    obs_mod.reset_audit_client()

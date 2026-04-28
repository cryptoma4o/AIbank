"""Конфигурация LLM-Gateway: YAML-routing + ENV-overrides.

YAML-схема — см. configs/default-routing.yaml.
Путь к конфигу настраивается переменной LLM_GATEWAY_CONFIG (по умолчанию
configs/default-routing.yaml относительно процесса).

ENV-overrides URL'ов для vLLM-backends (применяются после загрузки YAML):
- ``VLLM_GEMMA_URL``  → backend ``vllm-gemma``
- ``VLLM_QWEN_URL``   → backend ``vllm-qwen``
- ``VLLM_TPRO_URL``   → backend ``vllm-tpro``
- ``VLLM_BACKEND_URL_<NAME>`` → backend ``<name>`` (lowercased, kebab-case);
  generic форма для произвольных имён, чтобы не добавлять ENV-вариант на
  каждый новый backend. Пример: ``VLLM_BACKEND_URL_VLLM_VIKHR=...`` →
  ``vllm-vikhr``.

Применяются автоматически в ``get_routing_config()``. Это даёт возможность
переключить gateway на live-vLLM кластер через Helm values без правки YAML.
"""
from __future__ import annotations

import os
from pathlib import Path
from typing import Literal

import yaml
from pydantic import BaseModel, Field, model_validator
from pydantic_settings import BaseSettings


# ── Pydantic models for YAML routing config ────────────────────────────────────


class BackendConfig(BaseModel):
    type: Literal["openai_compatible", "mock"] = "openai_compatible"
    url: str | None = None
    timeout_s: float = 120.0

    @model_validator(mode="after")
    def _check_url_for_real_backend(self):
        if self.type == "openai_compatible" and not self.url:
            raise ValueError("openai_compatible backend requires url")
        return self


class ModelConfig(BaseModel):
    backend: str
    cost_per_1k_input_kop: int = 0
    cost_per_1k_output_kop: int = 0


class RoleConfig(BaseModel):
    model: str
    fallback: str | None = None


class RoutingConfig(BaseModel):
    version: int = 1
    backends: dict[str, BackendConfig]
    models: dict[str, ModelConfig]
    roles: dict[str, RoleConfig]

    @model_validator(mode="after")
    def _check_references(self):
        for model_name, model in self.models.items():
            if model.backend not in self.backends:
                raise ValueError(
                    f"model {model_name!r} references unknown backend {model.backend!r}"
                )
        for role_name, role in self.roles.items():
            if role.model not in self.models:
                raise ValueError(
                    f"role {role_name!r} references unknown model {role.model!r}"
                )
            if role.fallback and role.fallback not in self.models:
                raise ValueError(
                    f"role {role_name!r} fallback references unknown model {role.fallback!r}"
                )
        return self


def load_routing_config(path: str | Path) -> RoutingConfig:
    """Читает YAML-файл и валидирует через Pydantic."""
    raw = yaml.safe_load(Path(path).read_text(encoding="utf-8"))
    if not isinstance(raw, dict):
        raise ValueError(f"routing config at {path} must be a YAML mapping")
    return RoutingConfig.model_validate(raw)


# ── Process-level settings (env-driven) ────────────────────────────────────────


class Settings(BaseSettings):
    routing_config_path: str = Field(
        default="configs/default-routing.yaml",
        alias="LLM_GATEWAY_CONFIG",
    )
    log_requests: bool = Field(default=True, alias="LOG_REQUESTS")
    port: int = Field(default=8100, alias="PORT")
    # Если задано, ВСЕ запросы маршрутизируются на mock-backend.
    # Удобно в CI и eval-harness без реальных моделей.
    force_mock: bool = Field(default=False, alias="LLM_GATEWAY_FORCE_MOCK")
    # Tenant header — какое имя HTTP-заголовка читать.
    tenant_header: str = Field(default="X-Tenant-Id", alias="TENANT_HEADER")

    model_config = {"populate_by_name": True}


settings = Settings()


def find_config_path() -> Path:
    """Resolve routing config path relative to repo or working directory."""
    p = Path(settings.routing_config_path)
    if p.is_absolute() and p.exists():
        return p
    # 1) cwd/configured-path
    candidate = Path.cwd() / p
    if candidate.exists():
        return candidate
    # 2) module dir / configured-path (for `python -m gateway` cases)
    here = Path(__file__).resolve().parent.parent  # ai/llm-gateway/
    candidate = here / p
    if candidate.exists():
        return candidate
    # 3) env override fallback path
    if (here / "configs" / "default-routing.yaml").exists():
        return here / "configs" / "default-routing.yaml"
    raise FileNotFoundError(f"routing config not found at {p}")


# Optional module-level cache for app initialization. Lazily resolved.
_config_cache: RoutingConfig | None = None


# Mapping для коротких ENV-переменных. Дополняется generic VLLM_BACKEND_URL_<NAME>.
_VLLM_BACKEND_ENV_ALIASES: dict[str, str] = {
    "vllm-gemma": "VLLM_GEMMA_URL",
    "vllm-qwen": "VLLM_QWEN_URL",
    "vllm-tpro": "VLLM_TPRO_URL",
}


def apply_env_overrides(config: RoutingConfig) -> RoutingConfig:
    """Override URL для vLLM-backends из ENV.

    Семантика:
      * Mock-backend никогда не override'ится — он не имеет URL.
      * Если ENV не задан — оставляем YAML-значение (default).
      * Имя backend'а в ENV-варианте — kebab-case → SCREAMING_SNAKE_CASE
        (vllm-gemma → VLLM_BACKEND_URL_VLLM_GEMMA), плюс короткие алиасы.

    Возвращает новый RoutingConfig (без мутации входного), валидированный
    повторно через Pydantic.
    """
    raw = config.model_dump()
    backends = raw.get("backends", {})
    for name, backend in backends.items():
        if backend.get("type") != "openai_compatible":
            continue
        env_name = _VLLM_BACKEND_ENV_ALIASES.get(
            name,
            "VLLM_BACKEND_URL_" + name.upper().replace("-", "_"),
        )
        if env_url := os.environ.get(env_name):
            backend["url"] = env_url
    return RoutingConfig.model_validate(raw)


def get_routing_config() -> RoutingConfig:
    global _config_cache
    if _config_cache is None:
        loaded = load_routing_config(find_config_path())
        _config_cache = apply_env_overrides(loaded)
    return _config_cache


def reset_config_cache() -> None:
    """Только для тестов — сбрасывает кеш."""
    global _config_cache
    _config_cache = None

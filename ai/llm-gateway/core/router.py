"""Маршрутизатор моделей: разрешает `role:<name>` или прямое имя модели в backend."""
from __future__ import annotations

from dataclasses import dataclass

from . import config as _config_mod
from .backends import LLMBackend, build_backend
from .config import RoutingConfig


@dataclass(frozen=True)
class Resolution:
    """Результат резолвинга: модель + backend + опциональная fallback-модель."""

    requested: str         # то, что пришло от клиента (role:foo или model name)
    role: str | None       # если был role-prefix — имя роли
    model_name: str        # реальное имя модели для логов и cost
    backend: LLMBackend
    fallback_model: str | None
    cost_per_1k_input_kop: int
    cost_per_1k_output_kop: int


class ModelRouter:
    """Хранит backend-ы, разрешает роли и модели, отдаёт `Resolution`."""

    ROLE_PREFIX = "role:"

    def __init__(self, config: RoutingConfig, force_mock: bool = False) -> None:
        self.config = config
        self.force_mock = force_mock
        self._backends: dict[str, LLMBackend] = {}
        for name, bcfg in config.backends.items():
            self._backends[name] = build_backend(
                name=name, type_=bcfg.type, url=bcfg.url, timeout_s=bcfg.timeout_s
            )
        # Если включён force_mock и mock-backend ещё нет, создаём его на лету.
        if force_mock and "mock" not in self._backends:
            self._backends["mock"] = build_backend("mock", "mock", None, 0)

    def resolve(self, requested: str) -> Resolution | None:
        """Разрешает запрос:

        - `role:document-vision` → ищет в config.roles → берёт model + backend
        - `gemma-4-26b` → ищет в config.models напрямую
        - `force_mock` режим перенаправляет всё в mock backend (имя модели сохраняется)
        """
        role: str | None = None
        if requested.startswith(self.ROLE_PREFIX):
            role = requested[len(self.ROLE_PREFIX):]
            role_cfg = self.config.roles.get(role)
            if role_cfg is None:
                return None
            model_name = role_cfg.model
            fallback = role_cfg.fallback
        else:
            model_name = requested
            fallback = None

        model_cfg = self.config.models.get(model_name)
        if model_cfg is None:
            return None

        if self.force_mock:
            backend = self._backends["mock"]
        else:
            backend = self._backends.get(model_cfg.backend)
            if backend is None:
                return None

        return Resolution(
            requested=requested,
            role=role,
            model_name=model_name,
            backend=backend,
            fallback_model=fallback,
            cost_per_1k_input_kop=model_cfg.cost_per_1k_input_kop,
            cost_per_1k_output_kop=model_cfg.cost_per_1k_output_kop,
        )

    def fallback(self, fallback_model_name: str) -> Resolution | None:
        """Резолвит fallback-модель (без role prefix)."""
        return self.resolve(fallback_model_name)

    def list_models(self) -> list[str]:
        return list(self.config.models.keys())

    def list_roles(self) -> list[str]:
        return list(self.config.roles.keys())

    async def aclose(self) -> None:
        for b in self._backends.values():
            await b.aclose()


# Lazy module-level instance
_router: ModelRouter | None = None


def get_router() -> ModelRouter:
    global _router
    if _router is None:
        # Динамическое чтение settings — нужно для тестов, которые
        # пересоздают _config_mod.settings после правки env.
        _router = ModelRouter(
            _config_mod.get_routing_config(),
            force_mock=_config_mod.settings.force_mock,
        )
    return _router


def reset_router() -> None:
    """Только для тестов."""
    global _router
    _router = None

from __future__ import annotations
from .config import settings
from .backends import LLMBackend


class ModelRouter:
    def __init__(self) -> None:
        self._backends: dict[str, LLMBackend] = {}
        self._parse_config()

    def _parse_config(self) -> None:
        for entry in settings.model_backends.split(","):
            entry = entry.strip()
            if "=" not in entry:
                continue
            name, url = entry.split("=", 1)
            self._backends[name.strip()] = LLMBackend(name.strip(), url.strip())

    def get_backend(self, model: str) -> LLMBackend | None:
        # Exact match first
        if model in self._backends:
            return self._backends[model]
        # Prefix match (e.g. "gemma-4-27b" → "gemma-4")
        for name, backend in self._backends.items():
            if model.startswith(name):
                return backend
        # Default: first backend
        if self._backends:
            return next(iter(self._backends.values()))
        return None

    def list_models(self) -> list[str]:
        return list(self._backends.keys())

    async def aclose(self) -> None:
        for backend in self._backends.values():
            await backend.aclose()


router = ModelRouter()

"""Backend-реализации: OpenAI-совместимый upstream и Mock для тестов/eval."""
from __future__ import annotations

import abc
import hashlib
import json
import time
from typing import Any

import httpx


class LLMBackend(abc.ABC):
    """Абстрактный backend. Реализует chat completions."""

    name: str

    @abc.abstractmethod
    async def chat_completions(self, payload: dict[str, Any]) -> dict[str, Any]:
        ...

    async def aclose(self) -> None:  # noqa: D401 — default no-op
        return


class OpenAICompatibleBackend(LLMBackend):
    """HTTP-proxy к OpenAI-совместимому endpoint (vLLM, SGLang, OpenAI API)."""

    def __init__(self, name: str, base_url: str, timeout_s: float = 120.0) -> None:
        self.name = name
        self.base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(timeout=timeout_s)

    async def chat_completions(self, payload: dict[str, Any]) -> dict[str, Any]:
        resp = await self._client.post(
            f"{self.base_url}/v1/chat/completions", json=payload
        )
        resp.raise_for_status()
        return resp.json()

    async def aclose(self) -> None:
        await self._client.aclose()


class MockBackend(LLMBackend):
    """Детерминированный backend для CI и eval-harness без реальных моделей.

    Возвращает структурированный ответ, который зависит от хеша входного payload —
    одинаковый запрос даёт одинаковый ответ. Это полезно для регрессионных тестов:
    изменение в промпте → видимое изменение в ответе.
    """

    def __init__(self, name: str = "mock") -> None:
        self.name = name

    async def chat_completions(self, payload: dict[str, Any]) -> dict[str, Any]:
        messages = payload.get("messages", [])
        text_in = "\n".join(m.get("content", "") for m in messages if isinstance(m, dict))
        prompt_tokens = approx_tokens(text_in)

        digest = hashlib.sha256(text_in.encode("utf-8")).hexdigest()[:12]
        content = (
            f"[mock:{self.name}] echoing prompt digest={digest}; "
            f"intent={_guess_intent(text_in)}"
        )
        completion_tokens = approx_tokens(content)

        return {
            "id": f"mock-{digest}",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": payload.get("model", "mock"),
            "choices": [
                {
                    "index": 0,
                    "message": {"role": "assistant", "content": content},
                    "finish_reason": "stop",
                }
            ],
            "usage": {
                "prompt_tokens": prompt_tokens,
                "completion_tokens": completion_tokens,
                "total_tokens": prompt_tokens + completion_tokens,
            },
        }


def approx_tokens(text: str) -> int:
    """Грубая оценка количества токенов: ~1 токен на 4 символа.

    Реальные токенизаторы (tiktoken для GPT, sentencepiece для Gemma/Qwen)
    дают более точную оценку, но различаются между моделями. Для биллинга
    важна стабильная оценка верхнего порядка — приемлемо для MVP.
    """
    if not text:
        return 0
    return max(1, len(text) // 4)


def _guess_intent(text: str) -> str:
    """Эвристика для mock-ответа: сообщает, какую задачу 'распознал' mock."""
    lower = text.lower()
    if "паспорт" in lower or "passport" in lower:
        return "document-parsing"
    if "огрн" in lower or "устав" in lower:
        return "charter-extraction"
    if "уб" in lower or "учредител" in lower or "ubo" in lower:
        return "ubo-tracing"
    if "115-фз" in lower or "compliance" in lower or "регламент" in lower:
        return "compliance-rag"
    return "generic"


def build_backend(name: str, type_: str, url: str | None, timeout_s: float) -> LLMBackend:
    if type_ == "mock":
        return MockBackend(name=name)
    if type_ == "openai_compatible":
        if not url:
            raise ValueError(f"backend {name!r}: url required for openai_compatible")
        return OpenAICompatibleBackend(name=name, base_url=url, timeout_s=timeout_s)
    raise ValueError(f"unknown backend type {type_!r}")


def stable_payload_hash(payload: dict[str, Any]) -> str:
    return hashlib.sha256(
        json.dumps(payload, sort_keys=True, ensure_ascii=False).encode("utf-8")
    ).hexdigest()[:12]

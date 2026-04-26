from __future__ import annotations
import httpx
from typing import Any


class LLMBackend:
    def __init__(self, name: str, base_url: str) -> None:
        self.name = name
        self.base_url = base_url.rstrip("/")
        self._client = httpx.AsyncClient(timeout=120.0)

    async def chat_completions(self, payload: dict[str, Any]) -> dict[str, Any]:
        resp = await self._client.post(
            f"{self.base_url}/v1/chat/completions",
            json=payload,
        )
        resp.raise_for_status()
        return resp.json()

    async def list_models(self) -> dict[str, Any]:
        try:
            resp = await self._client.get(f"{self.base_url}/v1/models")
            return resp.json()
        except Exception:
            return {"object": "list", "data": []}

    async def aclose(self) -> None:
        await self._client.aclose()

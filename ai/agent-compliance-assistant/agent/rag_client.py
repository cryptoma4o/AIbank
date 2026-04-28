"""Async client for rag-service /v1/search.

В тестах подменяется через монкипатчинг или конструктор `Answerer`.
"""
from __future__ import annotations

import logging
import os
from typing import Any

import httpx

log = logging.getLogger(__name__)

DEFAULT_TIMEOUT_S = 15.0
RETRYABLE_STATUS = (500, 502, 503, 504)


def rag_url() -> str:
    return os.environ.get("RAG_SERVICE_URL", "http://localhost:8105").rstrip("/")


class RAGError(RuntimeError):
    """Raised when rag-service is unreachable or returns an error."""


class RAGClient:
    def __init__(
        self,
        base_url: str | None = None,
        timeout_s: float = DEFAULT_TIMEOUT_S,
        max_retries_5xx: int = 2,
    ) -> None:
        self.base_url = (base_url or rag_url()).rstrip("/")
        self.timeout_s = timeout_s
        self.max_retries_5xx = max_retries_5xx

    async def search(
        self,
        tenant_id: str,
        query: str,
        top_k: int = 5,
        source_type: str | None = None,
    ) -> list[dict[str, Any]]:
        """Return list of hits. Empty list on soft failure (RAG-сервис может быть выключен)."""
        url = f"{self.base_url}/v1/search"
        payload: dict[str, Any] = {
            "tenant_id": tenant_id,
            "query": query,
            "top_k": top_k,
        }
        if source_type:
            payload["filters"] = {"source_type": source_type}

        attempt = 0
        last_exc: Exception | None = None
        while attempt <= self.max_retries_5xx:
            attempt += 1
            try:
                async with httpx.AsyncClient(timeout=self.timeout_s) as client:
                    resp = await client.post(url, json=payload)
                if resp.status_code in RETRYABLE_STATUS and attempt <= self.max_retries_5xx:
                    log.warning(
                        "rag %d tenant=%s, retry %d/%d",
                        resp.status_code, tenant_id, attempt, self.max_retries_5xx,
                    )
                    continue
                if resp.status_code >= 400:
                    raise RAGError(
                        f"rag-service returned {resp.status_code}: {resp.text[:300]}"
                    )
                data = resp.json()
                hits = data.get("hits") if isinstance(data, dict) else data
                return list(hits or [])
            except httpx.HTTPError as exc:
                last_exc = exc
                log.warning("rag network error attempt=%d: %s", attempt, exc)
                if attempt > self.max_retries_5xx:
                    break
        raise RAGError(f"rag-service unreachable after {attempt} attempts: {last_exc}")

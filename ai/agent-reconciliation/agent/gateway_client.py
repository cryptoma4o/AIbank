"""HTTP client for llm-gateway with retry on 5xx and tenant header.

Все агенты разговаривают с gateway по одинаковому контракту:
- POST {LLM_GATEWAY_URL}/v1/chat/completions
- header X-Tenant-Id обязателен
- model = "role:<role>"
"""
from __future__ import annotations

import logging
import os
from typing import Any

import httpx

log = logging.getLogger(__name__)

DEFAULT_TIMEOUT_S = 60.0
DEFAULT_MAX_TOKENS = 1024
RETRYABLE_STATUS = (500, 502, 503, 504)


def gateway_url() -> str:
    return os.environ.get("LLM_GATEWAY_URL", "http://localhost:8100").rstrip("/")


class GatewayError(RuntimeError):
    """Raised when the gateway is unreachable or returns an error."""


class GatewayClient:
    """Thin async wrapper around the llm-gateway chat-completions endpoint.

    Retries: 2 attempts on 5xx / network errors, single attempt on 4xx.
    """

    def __init__(
        self,
        base_url: str | None = None,
        timeout_s: float = DEFAULT_TIMEOUT_S,
        max_retries_5xx: int = 2,
    ) -> None:
        self.base_url = (base_url or gateway_url()).rstrip("/")
        self.timeout_s = timeout_s
        self.max_retries_5xx = max_retries_5xx

    async def chat(
        self,
        role: str,
        messages: list[dict[str, str]],
        tenant_id: str,
        max_tokens: int = DEFAULT_MAX_TOKENS,
        temperature: float = 0.1,
    ) -> dict[str, Any]:
        if not tenant_id:
            raise GatewayError("tenant_id is required for gateway call")

        url = f"{self.base_url}/v1/chat/completions"
        payload = {
            "model": f"role:{role}",
            "messages": messages,
            "max_tokens": max_tokens,
            "temperature": temperature,
        }
        headers = {"X-Tenant-Id": tenant_id, "Content-Type": "application/json"}

        attempt = 0
        last_exc: Exception | None = None
        while attempt <= self.max_retries_5xx:
            attempt += 1
            try:
                async with httpx.AsyncClient(timeout=self.timeout_s) as client:
                    resp = await client.post(url, json=payload, headers=headers)
                if resp.status_code in RETRYABLE_STATUS and attempt <= self.max_retries_5xx:
                    log.warning(
                        "gateway %d for role=%s tenant=%s, retry %d/%d",
                        resp.status_code, role, tenant_id, attempt, self.max_retries_5xx,
                    )
                    continue
                if resp.status_code >= 400:
                    raise GatewayError(
                        f"gateway returned {resp.status_code}: {resp.text[:300]}"
                    )
                return resp.json()
            except httpx.HTTPError as exc:
                last_exc = exc
                log.warning(
                    "gateway network error role=%s attempt=%d: %s",
                    role, attempt, exc,
                )
                if attempt > self.max_retries_5xx:
                    break
        raise GatewayError(f"gateway unreachable after {attempt} attempts: {last_exc}")


def extract_content(response: dict[str, Any]) -> str:
    """Extract assistant content from a chat-completion response."""
    try:
        return response["choices"][0]["message"]["content"]
    except (KeyError, IndexError, TypeError) as exc:
        raise GatewayError(f"malformed gateway response: {exc!r}") from exc


def extract_metadata(response: dict[str, Any]) -> dict[str, Any]:
    """Extract aibank_gateway block (resolved model, fallback flag, latency)."""
    return dict(response.get("aibank_gateway") or {})

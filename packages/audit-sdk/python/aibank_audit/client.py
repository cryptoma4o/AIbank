"""Async HTTP client for the audit-service API.

Mirrors the Go SDK semantics:

- exponential backoff retries on 5xx and 429, never on 4xx;
- per-call timeout (default 5s);
- payload validation before the network hop;
- structured ``AuditAPIError`` carrying status + code + message.
"""

from __future__ import annotations

import asyncio
import json
from typing import Any

import httpx

from .types import AuditEvent, QueryOptions, RecordEventRequest


class AuditAPIError(Exception):
    """Raised for non-2xx audit-service responses."""

    def __init__(self, status_code: int, code: str = "", message: str = "") -> None:
        super().__init__(f"audit: {code or status_code}: {message}")
        self.status_code = status_code
        self.code = code
        self.message = message

    def is_retryable(self) -> bool:
        return self.status_code == 429 or 500 <= self.status_code < 600


class AsyncAuditClient:
    """Async client for the platform audit-service.

    Use as an async context manager so the underlying httpx connection
    pool is closed cleanly:

    >>> async with AsyncAuditClient(base_url="http://audit-service:8081") as c:
    ...     await c.append(req)
    """

    def __init__(
        self,
        *,
        base_url: str,
        timeout: float = 5.0,
        api_key: str | None = None,
        max_retries: int = 3,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        if not base_url:
            raise ValueError("audit: base_url is required")
        self._base_url = base_url.rstrip("/")
        self._timeout = timeout
        self._api_key = api_key
        self._max_retries = max(1, max_retries)
        self._owned_client = client is None
        headers = {"Accept": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._client = client or httpx.AsyncClient(timeout=timeout, headers=headers)

    async def __aenter__(self) -> "AsyncAuditClient":
        return self

    async def __aexit__(self, *_: object) -> None:
        await self.aclose()

    async def aclose(self) -> None:
        if self._owned_client:
            await self._client.aclose()

    async def append(self, request: RecordEventRequest) -> AuditEvent:
        """Append a new audit event via ``POST /v1/events``."""
        request.validate_required()
        body = request.model_dump_json()
        data = await self._do_with_retry(
            method="POST",
            path="/v1/events",
            content=body,
            content_type="application/json",
        )
        return AuditEvent.model_validate(data)

    async def list(self, query: QueryOptions) -> list[AuditEvent]:
        """List events via ``GET /v1/events`` with the given filters."""
        if not query.tenant_id:
            raise ValueError("audit: tenant_id is required")
        params: dict[str, str] = {"tenant_id": query.tenant_id}
        if query.entity_type:
            params["entity_type"] = query.entity_type
        if query.entity_id:
            params["entity_id"] = query.entity_id
        if query.limit:
            params["limit"] = str(query.limit)
        data = await self._do_with_retry(
            method="GET",
            path="/v1/events",
            params=params,
        )
        items = data.get("items") or []
        return [AuditEvent.model_validate(item) for item in items]

    async def _do_with_retry(
        self,
        *,
        method: str,
        path: str,
        params: dict[str, str] | None = None,
        content: str | None = None,
        content_type: str | None = None,
    ) -> dict[str, Any]:
        delay = 0.05
        last_exc: Exception | None = None
        for attempt in range(1, self._max_retries + 1):
            try:
                return await self._do(
                    method=method,
                    path=path,
                    params=params,
                    content=content,
                    content_type=content_type,
                )
            except AuditAPIError as exc:
                last_exc = exc
                if not exc.is_retryable():
                    raise
            except httpx.TransportError as exc:
                last_exc = exc

            if attempt == self._max_retries:
                break
            await asyncio.sleep(delay)
            delay *= 2

        assert last_exc is not None
        raise last_exc

    async def _do(
        self,
        *,
        method: str,
        path: str,
        params: dict[str, str] | None,
        content: str | None,
        content_type: str | None,
    ) -> dict[str, Any]:
        headers: dict[str, str] = {}
        if content_type:
            headers["Content-Type"] = content_type
        resp = await self._client.request(
            method,
            self._base_url + path,
            params=params,
            content=content,
            headers=headers,
        )
        body = resp.text
        if 200 <= resp.status_code < 300:
            if not body:
                return {}
            try:
                return json.loads(body)
            except json.JSONDecodeError as exc:
                raise AuditAPIError(
                    status_code=resp.status_code,
                    code="decode_error",
                    message=str(exc),
                ) from exc

        # Non-2xx — try to parse the writeError envelope from audit-service.
        code, message = "", body[:200]
        try:
            env = json.loads(body)
            err = env.get("error") or {}
            code = err.get("code") or ""
            message = err.get("message") or message
        except json.JSONDecodeError:
            pass
        raise AuditAPIError(status_code=resp.status_code, code=code, message=message)

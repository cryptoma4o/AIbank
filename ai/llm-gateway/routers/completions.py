"""Chat-completions endpoint, OpenAI-совместимый.

Расширения относительно стандартного OpenAI API:
- Заголовок `X-Tenant-Id` обязателен — учёт стоимости per tenant.
- `model` принимает значения вида `role:document-vision` для маршрутизации
  по логической роли.
- Заголовок `X-Agent-Role` (опционально) дублирует role-prefix; полезно для
  агентов, которые отправляют payload в OpenAI-совместимом виде с конкретным
  именем модели, но хотят использовать gateway-резолвинг.
"""
from __future__ import annotations

import logging
import time
from typing import Any

import httpx
from fastapi import APIRouter, Header, HTTPException
from pydantic import BaseModel

from core import config as _config_mod
from core.observability import emit_llm_completion
from core.router import get_router
from core.usage import tracker

log = logging.getLogger(__name__)
api = APIRouter()


class Message(BaseModel):
    role: str
    content: str


class ChatCompletionRequest(BaseModel):
    model: str
    messages: list[Message]
    temperature: float = 0.7
    max_tokens: int = 1024
    stream: bool = False


@api.post("/v1/chat/completions")
async def chat_completions(
    req: ChatCompletionRequest,
    x_tenant_id: str | None = Header(default=None, alias="X-Tenant-Id"),
    x_agent_role: str | None = Header(default=None, alias="X-Agent-Role"),
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> dict[str, Any]:
    if not x_tenant_id:
        raise HTTPException(status_code=400, detail="X-Tenant-Id header is required")

    requested = req.model
    # X-Agent-Role переопределяет model, если он указан и в model нет role-prefix.
    if x_agent_role and not requested.startswith("role:"):
        requested = f"role:{x_agent_role}"

    rt = get_router()
    resolution = rt.resolve(requested)
    if resolution is None:
        raise HTTPException(
            status_code=503,
            detail=f"No route available for {requested!r} "
                   f"(roles: {rt.list_roles()}, models: {rt.list_models()})",
        )

    payload = req.model_dump()
    payload["model"] = resolution.model_name

    start = time.monotonic()
    used_fallback = False
    try:
        result = await resolution.backend.chat_completions(payload)
    except httpx.HTTPError as exc:
        if resolution.fallback_model:
            log.warning(
                "primary backend %s failed for %s tenant=%s: %s — trying fallback %s",
                resolution.backend.name, resolution.model_name, x_tenant_id,
                exc, resolution.fallback_model,
            )
            fb = rt.fallback(resolution.fallback_model)
            if fb is None:
                raise HTTPException(status_code=502, detail="primary failed; fallback unresolved") from exc
            payload["model"] = fb.model_name
            try:
                result = await fb.backend.chat_completions(payload)
                resolution = fb
                used_fallback = True
            except httpx.HTTPError as exc2:
                raise HTTPException(
                    status_code=502, detail=f"primary and fallback failed: {exc2}"
                ) from exc2
        else:
            raise HTTPException(status_code=502, detail=f"backend error: {exc}") from exc

    elapsed = time.monotonic() - start
    usage = result.get("usage", {}) or {}
    prompt_tokens = int(usage.get("prompt_tokens", 0) or 0)
    completion_tokens = int(usage.get("completion_tokens", 0) or 0)

    tracker.record(
        tenant_id=x_tenant_id,
        model=resolution.model_name,
        role=resolution.role,
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        cost_per_1k_input_kop=resolution.cost_per_1k_input_kop,
        cost_per_1k_output_kop=resolution.cost_per_1k_output_kop,
    )

    if _config_mod.settings.log_requests:
        log.info(
            "tenant=%s role=%s model=%s backend=%s fallback=%s tokens_in=%d tokens_out=%d latency_ms=%.0f",
            x_tenant_id, resolution.role, resolution.model_name,
            resolution.backend.name, used_fallback,
            prompt_tokens, completion_tokens, elapsed * 1000,
        )

    result.setdefault("aibank_gateway", {})
    result["aibank_gateway"].update({
        "tenant_id": x_tenant_id,
        "role": resolution.role,
        "resolved_model": resolution.model_name,
        "backend": resolution.backend.name,
        "used_fallback": used_fallback,
        "latency_ms": round(elapsed * 1000, 2),
    })

    # ADR-0010: every successful LLM completion produces an audit event.
    # Best-effort — never block the response on audit-service availability.
    completion_id = str(result.get("id") or f"chat-{int(time.time() * 1000)}")
    await emit_llm_completion(
        tenant_id=x_tenant_id,
        completion_id=completion_id,
        actor_id=x_actor_id or "system",
        model=resolution.model_name,
        role=resolution.role,
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        backend=resolution.backend.name,
        used_fallback=used_fallback,
    )
    return result

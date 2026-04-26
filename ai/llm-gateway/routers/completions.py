from __future__ import annotations
import time
import logging
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Any
from core.router import router as model_router
from core.config import settings

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
async def chat_completions(req: ChatCompletionRequest) -> dict[str, Any]:
    backend = model_router.get_backend(req.model)
    if backend is None:
        raise HTTPException(status_code=503, detail=f"No backend available for model: {req.model}")

    start = time.monotonic()
    payload = req.model_dump()

    try:
        result = await backend.chat_completions(payload)
    except Exception as exc:
        log.error("Backend error for model %s: %s", req.model, exc)
        raise HTTPException(status_code=502, detail=f"Backend error: {exc}") from exc

    elapsed = time.monotonic() - start
    if settings.log_requests:
        log.info(
            "model=%s backend=%s tokens_in=%s tokens_out=%s latency_ms=%.0f",
            req.model,
            backend.name,
            result.get("usage", {}).get("prompt_tokens", "?"),
            result.get("usage", {}).get("completion_tokens", "?"),
            elapsed * 1000,
        )
    return result

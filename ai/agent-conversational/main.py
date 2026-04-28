"""Conversational agent FastAPI service.

Отвечает на вопросы клиента (юрлица/ИП) о процессе онбординга на русском
языке. Через llm-gateway role:ru-chat. Эскалирует сложные/жалобные кейсы
на менеджера.
"""
from __future__ import annotations

import logging

import uvicorn
from fastapi import FastAPI, Header, HTTPException

from agent import observability as obs
from agent.chat import handle_chat
from models.schemas import ChatRequest, ChatResponse

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

app = FastAPI(title="AIbank Conversational Agent", version="0.1.0")
obs.init_otel(app)


@app.post("/v1/chat", response_model=ChatResponse)
async def chat_endpoint(
    req: ChatRequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> ChatResponse:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    resp = await handle_chat(req)
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=req.session_id,
        actor_id=x_actor_id or "system",
        payload={
            "stage": getattr(req, "stage", None),
            "require_human_handoff": getattr(resp, "require_human_handoff", False),
        },
    )
    return resp


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "agent-conversational"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8104, reload=False)

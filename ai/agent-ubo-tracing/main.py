"""UBO tracing agent FastAPI service.

Строит граф владения по выпискам ЕГРЮЛ и определяет конечных бенефициаров
(UBOs) по 115-ФЗ (порог 25%). Использует llm-gateway role:ubo-tracing для
reasoning-цепочки.
"""
from __future__ import annotations

import logging

import uvicorn
from fastapi import FastAPI, Header, HTTPException

from agent import observability as obs
from agent.tracer import trace_ubo
from models.schemas import UBORequest, UBOResult

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

app = FastAPI(title="AIbank UBO Tracing Agent", version="0.1.0")
obs.init_otel(app)


@app.post("/v1/trace", response_model=UBOResult)
async def trace_endpoint(
    req: UBORequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> UBOResult:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    result = await trace_ubo(req)
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=req.application_id,
        actor_id=x_actor_id or "system",
        payload={
            "root_inn": getattr(req, "root_inn", None),
            "ubo_count": len(getattr(result, "ubos", []) or []),
            "confidence": getattr(result, "confidence", None),
        },
    )
    return result


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "agent-ubo-tracing"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8103, reload=False)

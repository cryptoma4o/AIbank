"""Reconciliation agent FastAPI service.

Сверяет данные, извлечённые из документов клиента (паспорт, устав), с
выпиской ЕГРЮЛ. Через llm-gateway (role:reconciliation) формирует список
расхождений и вопросов для уточнения.
"""
from __future__ import annotations

import logging

import uvicorn
from fastapi import FastAPI, Header, HTTPException

from agent import observability as obs
from agent.reconciler import reconcile
from models.schemas import ReconciliationRequest, ReconciliationResult

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
)

app = FastAPI(title="AIbank Reconciliation Agent", version="0.1.0")
obs.init_otel(app)


@app.post("/v1/reconcile", response_model=ReconciliationResult)
async def reconcile_endpoint(
    req: ReconciliationRequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> ReconciliationResult:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    result = await reconcile(
        extracted_data=req.extracted_data,
        egrul_data=req.egrul_data,
        tenant_id=req.tenant_id,
        application_id=req.application_id,
    )
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=req.application_id,
        actor_id=x_actor_id or "system",
        payload={
            "matches": getattr(result, "matches", None),
            "requires_review": getattr(result, "requires_review", None),
            "discrepancy_count": len(getattr(result, "discrepancies", []) or []),
        },
    )
    return result


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "agent-reconciliation"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8102, reload=False)

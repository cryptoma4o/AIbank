from __future__ import annotations

import logging
import os

import uvicorn
from fastapi import FastAPI, Header, HTTPException
from fastapi.responses import JSONResponse

from agent import observability as obs
from agent.scorer import score_application
from models.schemas import ScoringRequest, ScoringResponse

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(
    title="Agent Risk Scoring",
    version="0.1.0",
    description=(
        "Deterministic risk scoring (CatBoost-stub) + LLM-augmented explanation "
        "for AIbank onboarding pipeline. Scoring itself is not an LLM call (ADR-0011)."
    ),
)
obs.init_otel(app)


@app.get("/healthz")
async def healthz() -> JSONResponse:
    return JSONResponse({"status": "ok", "service": "agent-risk-scoring"})


@app.post("/v1/score", response_model=ScoringResponse)
async def score(
    req: ScoringRequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> ScoringResponse:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    log.info(
        "Scoring application_id=%s tenant_id=%s",
        req.application_id, req.tenant_id,
    )
    result = await score_application(req)
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=req.application_id,
        actor_id=x_actor_id or "system",
        payload={
            "recommendation": getattr(result, "recommendation", None),
            "blocked": getattr(result, "blocked", None),
            "model_used": getattr(result, "model_used", None),
        },
    )
    return result


if __name__ == "__main__":  # pragma: no cover
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.environ.get("PORT", "8104")),
        reload=False,
    )

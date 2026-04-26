from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from models.schemas import ScoringRequest, ScoringResponse
from agent.scorer import score_application

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

app = FastAPI(
    title="Agent Risk Scoring",
    version="0.1.0",
    description="LLM-augmented risk scoring agent for AIbank onboarding pipeline.",
)


@app.get("/healthz")
async def healthz() -> JSONResponse:
    return JSONResponse({"status": "ok"})


@app.post("/v1/score", response_model=ScoringResponse)
async def score(req: ScoringRequest) -> ScoringResponse:
    log.info("Scoring application_id=%s tenant_id=%s", req.application_id, req.tenant_id)
    return await score_application(req)


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8104, reload=False)

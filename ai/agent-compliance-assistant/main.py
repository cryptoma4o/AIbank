from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from models.schemas import ComplianceQuestion, ComplianceAnswer
from agent.assistant import answer

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

app = FastAPI(
    title="Compliance Assistant Agent",
    description="RAG-based Q&A for Russian banking regulations: 115-ФЗ, 375-П, 499-П, 152-ФЗ, 63-ФЗ",
    version="0.1.0",
)


@app.get("/healthz")
async def healthz() -> JSONResponse:
    return JSONResponse({"status": "ok", "service": "agent-compliance-assistant"})


@app.post("/v1/ask", response_model=ComplianceAnswer)
async def ask(req: ComplianceQuestion) -> ComplianceAnswer:
    log.info("Compliance question received: %s", req.question[:80])
    return await answer(req)


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8107, reload=False)

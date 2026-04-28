"""agent-compliance-assistant FastAPI app."""
from __future__ import annotations

import hashlib
import logging
import os

import uvicorn
from fastapi import FastAPI, Header, HTTPException

from agent import observability as obs
from agent.answerer import answer
from models.schemas import AnswerRequest, AnswerResponse

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(
    title="AIbank Compliance Assistant",
    description=(
        "RAG-based Q&A for Russian banking regulations: 115-ФЗ, 375-П, 499-П, "
        "590-П, 152-ФЗ. Always cites sources, never gives legal advice."
    ),
    version="0.1.0",
)
obs.init_otel(app)


@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "agent-compliance-assistant"}


@app.post("/v1/answer", response_model=AnswerResponse)
async def answer_question(
    req: AnswerRequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> AnswerResponse:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    if not req.question.strip():
        raise HTTPException(status_code=400, detail="question must not be empty")
    log.info(
        "compliance question tenant=%s top_k=%d source_type=%s",
        req.tenant_id, req.top_k, req.source_type,
    )
    result = await answer(req)
    # Question text is PII-adjacent — hash for the audit entity_id.
    entity_id = hashlib.sha256(req.question.strip().encode("utf-8")).hexdigest()[:16]
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=entity_id,
        actor_id=x_actor_id or "system",
        payload={
            "top_k": req.top_k,
            "source_type": req.source_type,
            "confidence": getattr(result, "confidence", None),
            "requires_human_review": getattr(result, "requires_human_review", None),
            "citation_count": len(getattr(result, "citations", []) or []),
        },
    )
    return result


if __name__ == "__main__":  # pragma: no cover
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.environ.get("PORT", "8106")),
        reload=False,
    )

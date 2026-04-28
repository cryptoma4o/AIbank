from __future__ import annotations

import logging
import os

import uvicorn
from fastapi import FastAPI, Header, HTTPException

from agent import observability as obs
from agent.extractor import extract_fields
from models.schemas import ExtractionRequest, ExtractionResult

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(title="AIbank Document Intake Agent", version="0.1.0")
obs.init_otel(app)


@app.post("/v1/extract", response_model=ExtractionResult)
async def extract(
    req: ExtractionRequest,
    x_actor_id: str | None = Header(default=None, alias="X-Actor-ID"),
) -> ExtractionResult:
    if not req.tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")
    result = await extract_fields(
        document_id=req.document_id,
        document_type=req.document_type,
        content_base64=req.content_base64,
        content_url=req.content_url,
        tenant_id=req.tenant_id,
    )
    await obs.emit_invocation(
        tenant_id=req.tenant_id,
        entity_id=req.document_id,
        actor_id=x_actor_id or "system",
        payload={
            "document_type": req.document_type,
            "model_used": getattr(result, "model_used", None),
            "field_count": len(getattr(result, "fields", []) or []),
        },
    )
    return result


@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "agent-document-intake"}


if __name__ == "__main__":  # pragma: no cover
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.environ.get("PORT", "8101")),
        reload=False,
    )

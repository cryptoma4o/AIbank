from __future__ import annotations
import logging
import uvicorn
from fastapi import FastAPI
from models.schemas import ExtractionRequest, ExtractionResult
from agent.extractor import extract_fields

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

app = FastAPI(title="AIbank Document Intake Agent", version="0.1.0")


@app.post("/v1/extract", response_model=ExtractionResult)
async def extract(req: ExtractionRequest) -> ExtractionResult:
    return await extract_fields(
        document_id=req.document_id,
        document_type=req.document_type,
        content_base64=req.content_base64,
        content_url=req.content_url,
    )


@app.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok", "service": "agent-document-intake"}


if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8101, reload=False)

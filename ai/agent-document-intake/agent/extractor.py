from __future__ import annotations
import json
import logging
import time
import httpx
from models.schemas import DocumentType, ExtractedField, ExtractionResult
from agent.ocr import extract_text
from agent.prompts import SYSTEM_PROMPT, FIELD_PROMPTS

log = logging.getLogger(__name__)

LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"


async def extract_fields(
    document_id: str,
    document_type: DocumentType,
    content_base64: str | None,
    content_url: str | None,
) -> ExtractionResult:
    start = time.monotonic()

    raw_text = await extract_text(content_base64, content_url)
    prompt = FIELD_PROMPTS.get(document_type, "Извлеки все доступные поля из документа в JSON.")

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": f"Документ:\n{raw_text}\n\n{prompt}"},
    ]

    model_used = DEFAULT_MODEL
    extracted: dict = {}

    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_GATEWAY_URL}/v1/chat/completions",
                json={"model": DEFAULT_MODEL, "messages": messages, "temperature": 0.1, "max_tokens": 512},
            )
            resp.raise_for_status()
            data = resp.json()
            content = data["choices"][0]["message"]["content"]
            extracted = json.loads(content)
    except json.JSONDecodeError:
        log.warning("LLM returned non-JSON for doc %s, using stub", document_id)
        extracted = {"_stub": "llm_unavailable"}
    except Exception as exc:
        log.warning("LLM gateway unavailable for doc %s: %s — using stub", document_id, exc)
        extracted = {"_stub": "llm_unavailable", "_error": str(exc)}
        model_used = "stub"

    fields = [
        ExtractedField(name=k, value=str(v), confidence=0.9 if model_used != "stub" else 0.0)
        for k, v in extracted.items()
    ]

    elapsed_ms = int((time.monotonic() - start) * 1000)
    return ExtractionResult(
        document_id=document_id,
        document_type=document_type,
        fields=fields,
        raw_text=raw_text[:500],
        model_used=model_used,
        processing_ms=elapsed_ms,
    )

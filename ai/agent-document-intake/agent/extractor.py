"""OCR + LLM document extractor with rule-based fallback.

Pipeline:
1. Run OCR (existing stub) → raw text.
2. Build a JSON-schema-style prompt for the document type and call llm-gateway
   with role:document-vision.
3. Parse JSON. On any failure (network, malformed JSON, empty content) we fall
   back to a deterministic rule-based extractor so the upstream pipeline keeps
   moving and the document is flagged for human review via low confidence.

JSON-schema-в-промпте подход выбран сознательно: gateway сейчас не
гарантирует function-calling/tools для всех бэкендов (см. ADR-0011, draft) и
все production-цели (gemma, qwen-vl) принимают строгие JSON-инструкции.
Когда gateway добавит structured-outputs (tool calling), достаточно будет
поменять только этот файл, формат данных остаётся.
"""
from __future__ import annotations

import json
import logging
import re
import time
from typing import Any

from agent.gateway_client import (
    GatewayClient,
    GatewayError,
    extract_content,
    extract_metadata,
)
from agent.ocr import extract_text
from agent.prompts import FIELD_PROMPTS, SYSTEM_PROMPT
from models.schemas import (
    DocumentType,
    ExtractedField,
    ExtractionResult,
)

log = logging.getLogger(__name__)

ROLE = "document-vision"
LLM_CONFIDENCE = 0.9
RULE_CONFIDENCE = 0.4    # rule-based hits are useful but always low confidence


# ---------- LLM JSON parsing ---------------------------------------------------

_JSON_BLOCK_RE = re.compile(r"\{.*\}", re.DOTALL)


def _parse_llm_json(content: str) -> dict[str, Any]:
    """Tolerant JSON parse: strip code fences, fall back to first/last brace."""
    text = (content or "").strip()
    if text.startswith("```"):
        text = text.strip("`")
        if text.lower().startswith("json"):
            text = text[4:]
        text = text.strip()
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError:
        m = _JSON_BLOCK_RE.search(text)
        if not m:
            raise
        parsed = json.loads(m.group(0))
    if not isinstance(parsed, dict):
        raise ValueError("LLM returned non-object JSON")
    return parsed


def _fields_from_dict(data: dict[str, Any], confidence: float) -> list[ExtractedField]:
    out: list[ExtractedField] = []
    for k, v in data.items():
        if k.startswith("_"):
            continue
        if v is None:
            continue
        if isinstance(v, (list, dict)):
            value_str = json.dumps(v, ensure_ascii=False)
        else:
            value_str = str(v)
        if not value_str:
            continue
        out.append(ExtractedField(name=str(k), value=value_str, confidence=confidence))
    return out


# ---------- Rule-based fallback -----------------------------------------------

# Прим.: rule-based — не замена LLM, а seatbelt: даёт хоть какие-то поля,
# чтобы UI/operator знали, что OCR прошёл, а LLM упал. Не пытаемся быть умными.

_INN_RE = re.compile(r"\b(\d{10}|\d{12})\b")
_OGRN_RE = re.compile(r"\b(\d{13}|\d{15})\b")
_KPP_RE = re.compile(r"\b\d{9}\b")
_BIK_RE = re.compile(r"\b04\d{7}\b")
_DATE_RE = re.compile(r"\b(\d{2})[.\-/](\d{2})[.\-/](\d{4})\b")
_PASSPORT_SERIES_NUMBER_RE = re.compile(r"\b(\d{2}\s?\d{2})\s+(\d{6})\b")
_DEPARTMENT_CODE_RE = re.compile(r"\b\d{3}-\d{3}\b")


def _normalize_date(s: str) -> str:
    m = _DATE_RE.search(s)
    if not m:
        return s
    dd, mm, yyyy = m.groups()
    return f"{yyyy}-{mm}-{dd}"


def _rule_based_extract(doc_type: DocumentType, text: str) -> dict[str, Any]:
    if not text:
        return {}
    extracted: dict[str, Any] = {}
    if doc_type == DocumentType.PASSPORT:
        m = _PASSPORT_SERIES_NUMBER_RE.search(text)
        if m:
            extracted["series"] = m.group(1).replace(" ", "")
            extracted["number"] = m.group(2)
        m = _DEPARTMENT_CODE_RE.search(text)
        if m:
            extracted["department_code"] = m.group(0)
        m = _DATE_RE.search(text)
        if m:
            extracted["birth_date"] = _normalize_date(m.group(0))
        return extracted

    if doc_type in (
        DocumentType.EGRUL,
        DocumentType.INN_CERTIFICATE,
        DocumentType.OGRN_CERTIFICATE,
        DocumentType.CHARTER,
    ):
        m = _INN_RE.search(text)
        if m:
            extracted["inn"] = m.group(1)
        m = _OGRN_RE.search(text)
        if m:
            extracted["ogrn"] = m.group(1)
        m = _KPP_RE.search(text)
        if m and m.group(0) != extracted.get("inn"):
            extracted["kpp"] = m.group(0)
        return extracted

    if doc_type == DocumentType.BANK_STATEMENT:
        m = _BIK_RE.search(text)
        if m:
            extracted["bik"] = m.group(0)
        m = _INN_RE.search(text)
        if m:
            extracted["inn"] = m.group(1)
        return extracted

    return extracted


# ---------- Public API --------------------------------------------------------


async def extract_fields(
    document_id: str,
    document_type: DocumentType,
    content_base64: str | None,
    content_url: str | None,
    tenant_id: str = "",
    *,
    client: GatewayClient | None = None,
) -> ExtractionResult:
    """Run OCR + LLM extraction; gracefully fall back to rule-based on failure."""
    start = time.monotonic()

    raw_text = await extract_text(content_base64, content_url)
    field_prompt = FIELD_PROMPTS.get(
        document_type, "Извлеки все доступные поля из документа в JSON."
    )

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {
            "role": "user",
            "content": (
                f"Документ ({document_type.value}):\n{raw_text}\n\n"
                f"{field_prompt}\n\n"
                "Верни строго валидный JSON-объект без пояснений и без code-fence."
            ),
        },
    ]

    fields: list[ExtractedField] = []
    metadata: dict[str, Any] = {"document_id": document_id, "role": ROLE}
    model_used = "stub"

    # Tenant_id is required by gateway. Если не передали — fallback сразу.
    if not tenant_id:
        log.info("tenant_id not provided for doc=%s — skipping LLM call", document_id)
        metadata["llm_skipped"] = "missing_tenant"
    else:
        gw = client or GatewayClient()
        try:
            response = await gw.chat(
                role=ROLE,
                messages=messages,
                tenant_id=tenant_id,
                max_tokens=800,
                temperature=0.0,
            )
            metadata.update(extract_metadata(response))
            content = extract_content(response)
            parsed = _parse_llm_json(content)
            fields = _fields_from_dict(parsed, confidence=LLM_CONFIDENCE)
            if fields:
                model_used = str(metadata.get("resolved_model") or "llm")
        except (GatewayError, json.JSONDecodeError, ValueError) as exc:
            log.warning(
                "LLM extraction failed for doc=%s: %s — falling back to rules",
                document_id, exc,
            )
            metadata["llm_error"] = str(exc)

    if not fields:
        # Fallback path: rule-based regex extractor on OCR output.
        rule_data = _rule_based_extract(document_type, raw_text)
        fields = _fields_from_dict(rule_data, confidence=RULE_CONFIDENCE)
        if fields and model_used == "stub":
            model_used = "rule-based"

    elapsed_ms = int((time.monotonic() - start) * 1000)
    return ExtractionResult(
        document_id=document_id,
        document_type=document_type,
        fields=fields,
        raw_text=raw_text[:500],
        model_used=model_used,
        processing_ms=elapsed_ms,
        gateway_metadata=metadata,
    )

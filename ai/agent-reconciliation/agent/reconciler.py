"""Reconciliation logic: deterministic pre-check + LLM augmentation."""
from __future__ import annotations

import json
import logging
from typing import Any

from agent.gateway_client import (
    GatewayClient,
    GatewayError,
    extract_content,
    extract_metadata,
)
from agent.prompts import FEW_SHOT_EXAMPLE, SYSTEM_PROMPT, USER_PROMPT_TEMPLATE
from models.schemas import Discrepancy, ReconciliationResult, Severity

log = logging.getLogger(__name__)

ROLE = "reconciliation"

# Поля, которые сверяем эвристикой ДО обращения к LLM — даёт детерминированный
# базовый результат и снижает зависимость от качества модели для очевидных кейсов.
KEY_FIELDS_HIGH = ("inn", "ogrn", "director", "ceo")
KEY_FIELDS_MEDIUM = ("name", "company_name", "address", "legal_address")
KEY_FIELDS_LOW = ("okved", "phone", "email")

_SEVERITY_BY_FIELD: dict[str, Severity] = {
    **{f: Severity.HIGH for f in KEY_FIELDS_HIGH},
    **{f: Severity.MEDIUM for f in KEY_FIELDS_MEDIUM},
    **{f: Severity.LOW for f in KEY_FIELDS_LOW},
}


def _normalize(v: Any) -> str:
    if v is None:
        return ""
    return str(v).strip().lower()


def _heuristic_discrepancies(
    extracted: dict[str, Any], registry: dict[str, Any]
) -> list[Discrepancy]:
    """Detect structural mismatches in well-known fields without LLM."""
    found: list[Discrepancy] = []
    seen: set[str] = set()
    for field, severity in _SEVERITY_BY_FIELD.items():
        if field in seen:
            continue
        ev = extracted.get(field)
        rv = registry.get(field)
        if ev is None or rv is None:
            continue
        if _normalize(ev) != _normalize(rv):
            found.append(
                Discrepancy(
                    field=field,
                    extracted_value=str(ev),
                    registry_value=str(rv),
                    severity=severity,
                )
            )
            seen.add(field)
    return found


def _parse_llm_payload(content: str) -> dict[str, Any]:
    """Tolerant JSON parsing — handle code-fenced answers from chat models."""
    text = content.strip()
    if text.startswith("```"):
        text = text.strip("`")
        # remove optional language hint
        if text.lower().startswith("json"):
            text = text[4:]
        text = text.strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        # Fall back to first-{ to last-} slice
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            try:
                return json.loads(text[start : end + 1])
            except json.JSONDecodeError:
                pass
        raise


def _coerce_discrepancies(items: Any) -> list[Discrepancy]:
    out: list[Discrepancy] = []
    if not isinstance(items, list):
        return out
    for raw in items:
        if not isinstance(raw, dict):
            continue
        try:
            out.append(
                Discrepancy(
                    field=str(raw.get("field", "")),
                    extracted_value=str(raw.get("extracted_value", "")),
                    registry_value=str(raw.get("registry_value", "")),
                    severity=Severity(str(raw.get("severity", "medium")).lower()),
                )
            )
        except (ValueError, TypeError) as exc:
            log.warning("skip malformed discrepancy: %s (%s)", raw, exc)
    return out


def _merge_discrepancies(
    heuristic: list[Discrepancy], llm: list[Discrepancy]
) -> list[Discrepancy]:
    by_field: dict[str, Discrepancy] = {d.field: d for d in heuristic}
    for d in llm:
        if d.field not in by_field:
            by_field[d.field] = d
    return list(by_field.values())


async def reconcile(
    extracted_data: dict[str, Any],
    egrul_data: dict[str, Any],
    tenant_id: str,
    application_id: str,
    client: GatewayClient | None = None,
) -> ReconciliationResult:
    heuristic = _heuristic_discrepancies(extracted_data, egrul_data)

    user_prompt = USER_PROMPT_TEMPLATE.format(
        extracted=json.dumps(extracted_data, ensure_ascii=False, sort_keys=True),
        registry=json.dumps(egrul_data, ensure_ascii=False, sort_keys=True),
    )
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT + "\n\n" + FEW_SHOT_EXAMPLE},
        {"role": "user", "content": user_prompt},
    ]

    gw = client or GatewayClient()
    questions: list[str] = []
    llm_discrepancies: list[Discrepancy] = []
    metadata: dict[str, Any] = {"application_id": application_id}
    llm_matches: bool | None = None
    llm_requires_review: bool | None = None

    try:
        response = await gw.chat(
            role=ROLE,
            messages=messages,
            tenant_id=tenant_id,
            max_tokens=800,
        )
        metadata.update(extract_metadata(response))
        payload = _parse_llm_payload(extract_content(response))
        llm_matches = bool(payload.get("matches", False))
        llm_requires_review = bool(payload.get("requires_review", False))
        llm_discrepancies = _coerce_discrepancies(payload.get("discrepancies"))
        raw_questions = payload.get("questions_for_client") or []
        if isinstance(raw_questions, list):
            questions = [str(q) for q in raw_questions if str(q).strip()]
    except (GatewayError, json.JSONDecodeError, ValueError) as exc:
        log.warning("LLM reconcile failed for app=%s: %s", application_id, exc)
        metadata["llm_error"] = str(exc)

    discrepancies = _merge_discrepancies(heuristic, llm_discrepancies)
    matches = len(discrepancies) == 0 if llm_matches is None else (
        llm_matches and not discrepancies
    )
    has_high = any(d.severity == Severity.HIGH for d in discrepancies)
    requires_review = has_high if llm_requires_review is None else (
        llm_requires_review or has_high
    )

    if not questions and discrepancies:
        # Fallback questions if LLM didn't produce any.
        questions = [
            f"Уточните поле «{d.field}»: в документах — «{d.extracted_value}», "
            f"в реестре — «{d.registry_value}»."
            for d in discrepancies
        ]

    return ReconciliationResult(
        matches=matches,
        discrepancies=discrepancies,
        questions_for_client=questions,
        requires_review=requires_review,
        gateway_metadata=metadata,
    )

"""RAG-based compliance answering pipeline.

Pipeline:
1. RAG-search relevant snippets via rag-service.
2. Build prompt with retrieved snippets + question.
3. Call llm-gateway with role:rag.
4. Parse to structured AnswerResponse with citations.
"""
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
from agent.prompts import (
    DISCLAIMER,
    SYSTEM_PROMPT,
    USER_PROMPT_TEMPLATE,
    looks_like_injection,
)
from agent.rag_client import RAGClient, RAGError
from models.schemas import AnswerRequest, AnswerResponse, Citation

log = logging.getLogger(__name__)

ROLE = "rag"


def _format_context(citations: list[Citation]) -> str:
    if not citations:
        return "(нормативная база не найдена для данного запроса)"
    return "\n\n".join(
        f"[{c.doc_id}] {c.source}: {c.snippet}" for c in citations
    )


def _format_case_context(context: dict[str, Any]) -> str:
    if not context:
        return "(пусто)"
    try:
        return json.dumps(context, ensure_ascii=False, indent=2, sort_keys=True)
    except (TypeError, ValueError):
        return str(context)


def _hits_to_citations(hits: list[dict[str, Any]]) -> list[Citation]:
    out: list[Citation] = []
    for h in hits:
        try:
            out.append(
                Citation(
                    doc_id=str(h.get("document_id") or h.get("doc_id") or ""),
                    snippet=str(h.get("snippet", "")),
                    source=str(h.get("source", "")),
                    source_type=str(h.get("source_type", "law") or "law"),
                    score=float(h.get("score", 0.0) or 0.0),
                )
            )
        except (TypeError, ValueError) as exc:
            log.warning("skip malformed RAG hit: %s (%s)", h, exc)
    return out


def _confidence(citations: list[Citation], used_llm: bool) -> str:
    if citations and used_llm:
        return "high"
    if citations and not used_llm:
        return "medium"
    if not citations and used_llm:
        return "medium"
    return "low"


async def answer(
    req: AnswerRequest,
    *,
    rag: RAGClient | None = None,
    gateway: GatewayClient | None = None,
) -> AnswerResponse:
    if not req.tenant_id:
        raise ValueError("tenant_id is required")
    if not req.question.strip():
        raise ValueError("question must not be empty")

    metadata: dict[str, Any] = {}
    citations: list[Citation] = []

    # 1) Retrieve context — fail-soft on RAG outage
    rag_client = rag or RAGClient()
    try:
        hits = await rag_client.search(
            tenant_id=req.tenant_id,
            query=req.question,
            top_k=max(1, min(req.top_k, 10)),
            source_type=req.source_type,
        )
        citations = _hits_to_citations(hits)
    except RAGError as exc:
        log.warning("RAG unavailable: %s", exc)
        metadata["rag_error"] = str(exc)

    # 2/3) Hard guard: prompt-injection detection — short-circuit без LLM
    injection = looks_like_injection(req.question)
    if injection:
        metadata["short_circuit"] = "injection_detected"
        return AnswerResponse(
            tenant_id=req.tenant_id,
            question=req.question,
            answer=(
                "В вашем сообщении обнаружены инструкции, выходящие за рамки "
                "нормативно-правовых консультаций. Я отвечаю только на вопросы "
                "по российскому банковскому законодательству на основании "
                "предоставленной нормативной базы. Передаю запрос на ручное "
                "рассмотрение комплаенс-офицеру."
            ),
            citations=citations,
            confidence="low",
            requires_human_review=True,
            disclaimer=DISCLAIMER,
            gateway_metadata=metadata,
        )

    # 2/3) Build prompt and call LLM
    user_prompt = USER_PROMPT_TEMPLATE.format(
        context=_format_context(citations),
        case_context=_format_case_context(req.context),
        question=req.question.strip(),
    )
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": user_prompt},
    ]

    answer_text = ""
    used_llm = False
    gw = gateway or GatewayClient()
    try:
        response = await gw.chat(
            role=ROLE,
            messages=messages,
            tenant_id=req.tenant_id,
            max_tokens=900,
            temperature=0.1,
        )
        metadata.update(extract_metadata(response))
        answer_text = extract_content(response).strip()
        used_llm = bool(answer_text)
    except GatewayError as exc:
        log.warning("LLM gateway unavailable: %s", exc)
        metadata["llm_error"] = str(exc)

    # 4) Fallback if LLM produced nothing
    if not answer_text:
        if citations:
            top = citations[0]
            answer_text = (
                "Сервис LLM временно недоступен. По запросу найдены релевантные "
                f"нормативные источники, ключевой — [{top.doc_id}] {top.source}: "
                f"{top.snippet}"
            )
        else:
            answer_text = (
                "Сервис LLM временно недоступен и в нормативной базе не найдено "
                "релевантных источников. Обратитесь к нормативной документации "
                "напрямую или повторите запрос позже."
            )

    requires_review = (not citations) or (not used_llm)

    return AnswerResponse(
        tenant_id=req.tenant_id,
        question=req.question,
        answer=answer_text,
        citations=citations,
        confidence=_confidence(citations, used_llm),
        requires_human_review=requires_review,
        disclaimer=DISCLAIMER,
        gateway_metadata=metadata,
    )

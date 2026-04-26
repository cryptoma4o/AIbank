from __future__ import annotations
import logging
import httpx
from models.schemas import ComplianceQuestion, ComplianceAnswer, ComplianceSource
from agent.prompts import SYSTEM_PROMPT, DISCLAIMER

log = logging.getLogger(__name__)
RAG_URL = "http://rag-service:8105"
LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"


async def answer(req: ComplianceQuestion) -> ComplianceAnswer:
    sources: list[ComplianceSource] = []

    # Step 1: retrieve relevant chunks from RAG service
    try:
        async with httpx.AsyncClient(timeout=15.0) as client:
            rag_payload = {"query": req.question, "top_k": req.top_k}
            if req.regulation_filter:
                rag_payload["source_filter"] = req.regulation_filter
            resp = await client.post(f"{RAG_URL}/v1/search", json=rag_payload)
            if resp.status_code == 200:
                for item in resp.json():
                    sources.append(ComplianceSource(**item))
    except Exception as exc:
        log.warning("RAG service unavailable: %s", exc)

    # Step 2: build context from sources
    context_parts = [f"[{s.source.upper()}] {s.chunk}" for s in sources]
    context = "\n\n".join(context_parts) if context_parts else "Нормативная база недоступна."

    # Step 3: add application context if provided
    app_context = ""
    if req.context:
        app_context = f"\n\nКонтекст заявки: {req.context}"

    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": (
            f"Нормативный контекст:\n{context}{app_context}\n\n"
            f"Вопрос: {req.question}"
        )},
    ]

    answer_text = ""
    model_used = DEFAULT_MODEL
    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_GATEWAY_URL}/v1/chat/completions",
                json={"model": DEFAULT_MODEL, "messages": messages,
                      "temperature": 0.1, "max_tokens": 800},
            )
            resp.raise_for_status()
            answer_text = resp.json()["choices"][0]["message"]["content"].strip()
    except Exception as exc:
        log.warning("LLM unavailable: %s", exc)
        answer_text = ("Сервис LLM временно недоступен. "
                       "Обратитесь к нормативной документации напрямую.")
        model_used = "stub"

    confidence = "high" if sources else ("medium" if model_used != "stub" else "low")

    return ComplianceAnswer(
        question=req.question,
        answer=answer_text,
        sources=sources,
        confidence=confidence,
        disclaimer=DISCLAIMER,
        model_used=model_used,
    )

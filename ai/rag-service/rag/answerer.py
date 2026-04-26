from __future__ import annotations
import logging
import httpx
from models.schemas import AskRequest, AskResponse, SearchResult
from rag.retriever import search, SearchRequest

log = logging.getLogger(__name__)
LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"

SYSTEM_PROMPT = (
    "Ты — эксперт по российскому банковскому законодательству и комплаенс. "
    "Отвечай точно, со ссылками на нормативные акты. "
    "Используй только предоставленный контекст."
)


async def ask(req: AskRequest) -> AskResponse:
    search_req = SearchRequest(query=req.question, top_k=req.top_k, source_filter=req.source_filter)
    sources: list[SearchResult] = await search(search_req)

    context = "\n\n".join(f"[{s.source}] {s.chunk}" for s in sources) if sources else "Контекст недоступен."
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": f"Контекст:\n{context}\n\nВопрос: {req.question}"},
    ]

    model_used = DEFAULT_MODEL
    answer = ""
    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_GATEWAY_URL}/v1/chat/completions",
                json={"model": DEFAULT_MODEL, "messages": messages, "temperature": 0.2, "max_tokens": 600},
            )
            resp.raise_for_status()
            answer = resp.json()["choices"][0]["message"]["content"].strip()
    except Exception as exc:
        log.warning("LLM answering failed: %s", exc)
        answer = "Сервис LLM недоступен. Обратитесь к нормативной документации напрямую."
        model_used = "stub"

    return AskResponse(answer=answer, sources=sources, model_used=model_used)

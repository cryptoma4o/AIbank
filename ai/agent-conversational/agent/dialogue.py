from __future__ import annotations
import logging
import httpx
from models.schemas import ChatRequest, ChatResponse, Message, OnboardingStage
from agent.prompts import BASE_SYSTEM, STAGE_PROMPTS, ESCALATION_TRIGGER_WORDS

log = logging.getLogger(__name__)
LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"


def _needs_escalation(text: str) -> bool:
    lower = text.lower()
    return any(word in lower for word in ESCALATION_TRIGGER_WORDS)


async def chat(req: ChatRequest) -> ChatResponse:
    stage_prompt = STAGE_PROMPTS.get(req.stage, "")
    system_content = f"{BASE_SYSTEM}\n\n{stage_prompt}"

    messages = [{"role": "system", "content": system_content}]
    for msg in req.history[-10:]:  # keep last 10 turns
        messages.append({"role": msg.role, "content": msg.content})
    messages.append({"role": "user", "content": req.user_message})

    assistant_message = ""
    model_used = DEFAULT_MODEL
    escalate = _needs_escalation(req.user_message)

    if not escalate:
        try:
            async with httpx.AsyncClient(timeout=60.0) as client:
                resp = await client.post(
                    f"{LLM_GATEWAY_URL}/v1/chat/completions",
                    json={"model": DEFAULT_MODEL, "messages": messages,
                          "temperature": 0.5, "max_tokens": 400},
                )
                resp.raise_for_status()
                assistant_message = resp.json()["choices"][0]["message"]["content"].strip()
                if _needs_escalation(assistant_message):
                    escalate = True
        except Exception as exc:
            log.warning("LLM unavailable: %s", exc)
            assistant_message = ("Извините, сервис временно недоступен. "
                                 "Пожалуйста, свяжитесь с менеджером банка.")
            model_used = "stub"

    if escalate:
        assistant_message = ("Ваш вопрос требует участия специалиста. "
                             "Менеджер банка свяжется с вами в течение 30 минут.")

    updated_history = list(req.history) + [
        Message(role="user", content=req.user_message),
        Message(role="assistant", content=assistant_message),
    ]

    suggested_action = None
    if req.stage == OnboardingStage.DOCUMENT_COLLECTION:
        suggested_action = "upload_document"
    elif escalate:
        suggested_action = "call_manager"

    return ChatResponse(
        session_id=req.session_id,
        assistant_message=assistant_message,
        updated_history=updated_history,
        needs_escalation=escalate,
        suggested_action=suggested_action,
        model_used=model_used,
    )

"""Chat orchestration: pre-checks → LLM → post-processing."""
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
    ESCALATION_REPLY,
    ESCALATION_TRIGGERS,
    OFF_TOPIC_REPLY,
    OFF_TOPIC_TRIGGERS,
    build_system_prompt,
)
from models.schemas import ChatRequest, ChatResponse, OnboardingStage

log = logging.getLogger(__name__)

ROLE = "ru-chat"


def _matches_any(text: str, needles: tuple[str, ...]) -> bool:
    lower = text.lower()
    return any(n in lower for n in needles)


def needs_escalation(text: str) -> bool:
    return _matches_any(text, ESCALATION_TRIGGERS)


def is_off_topic(text: str) -> bool:
    return _matches_any(text, OFF_TOPIC_TRIGGERS)


def _parse_llm_payload(content: str) -> dict[str, Any]:
    text = content.strip()
    if text.startswith("```"):
        text = text.strip("`")
        if text.lower().startswith("json"):
            text = text[4:]
        text = text.strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return json.loads(text[start : end + 1])
        # Plain text — wrap into a minimal payload.
        return {"response": text, "suggested_next_steps": [],
                "require_human_handoff": False}


def _default_next_steps(stage: OnboardingStage) -> list[str]:
    if stage == OnboardingStage.INITIAL:
        return ["Подготовьте документы", "Перейдите к шагу загрузки"]
    if stage == OnboardingStage.DOCUMENTS:
        return ["Загрузите паспорт", "Загрузите выписку ЕГРЮЛ"]
    if stage == OnboardingStage.VERIFICATION:
        return ["Дождитесь уведомления", "Проверьте email"]
    if stage == OnboardingStage.FINALIZATION:
        return ["Подпишите договор", "Получите реквизиты счёта"]
    return []


async def handle_chat(
    req: ChatRequest, client: GatewayClient | None = None
) -> ChatResponse:
    metadata: dict[str, Any] = {"session_id": req.session_id}

    # Hard guards before touching LLM.
    if needs_escalation(req.message):
        return ChatResponse(
            response=ESCALATION_REPLY,
            suggested_next_steps=["Дождитесь звонка менеджера"],
            require_human_handoff=True,
            gateway_metadata={**metadata, "short_circuit": "escalation"},
        )
    if is_off_topic(req.message):
        return ChatResponse(
            response=OFF_TOPIC_REPLY,
            suggested_next_steps=_default_next_steps(req.stage),
            require_human_handoff=False,
            gateway_metadata={**metadata, "short_circuit": "off_topic"},
        )

    system_prompt = build_system_prompt(req.stage, req.context)
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": req.message},
    ]

    gw = client or GatewayClient()
    try:
        response = await gw.chat(
            role=ROLE,
            messages=messages,
            tenant_id=req.tenant_id,
            max_tokens=400,
            temperature=0.4,
        )
        metadata.update(extract_metadata(response))
        payload = _parse_llm_payload(extract_content(response))
    except (GatewayError, json.JSONDecodeError, ValueError) as exc:
        log.warning("LLM chat failed session=%s: %s", req.session_id, exc)
        metadata["llm_error"] = str(exc)
        return ChatResponse(
            response=(
                "Извините, сервис временно недоступен. Попробуйте позже или "
                "оставьте обращение — менеджер свяжется с вами."
            ),
            suggested_next_steps=_default_next_steps(req.stage),
            require_human_handoff=True,
            gateway_metadata=metadata,
        )

    answer = str(payload.get("response", "")).strip()
    if not answer:
        answer = (
            "Не удалось сформировать ответ. Передам обращение менеджеру."
        )
        require_handoff = True
    else:
        require_handoff = bool(payload.get("require_human_handoff", False))

    raw_steps = payload.get("suggested_next_steps") or []
    if isinstance(raw_steps, list):
        steps = [str(s) for s in raw_steps if str(s).strip()]
    else:
        steps = []
    if not steps:
        steps = _default_next_steps(req.stage)

    # Defence-in-depth: если в ответе LLM проскочили запрещённые формулировки —
    # эскалируем.
    if needs_escalation(answer):
        require_handoff = True

    return ChatResponse(
        response=answer,
        suggested_next_steps=steps,
        require_human_handoff=require_handoff,
        gateway_metadata=metadata,
    )

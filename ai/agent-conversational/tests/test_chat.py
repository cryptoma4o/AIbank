"""Unit tests for the conversational chat agent."""
from __future__ import annotations

import json
from typing import Any

import pytest

from agent import chat as chat_mod
from agent.chat import handle_chat
from models.schemas import ChatRequest, OnboardingStage


class FakeGateway:
    def __init__(self, payload: dict[str, Any] | None = None,
                 raise_exc: Exception | None = None) -> None:
        self.payload = payload
        self.raise_exc = raise_exc
        self.calls: list[dict[str, Any]] = []

    async def chat(self, role: str, messages: list[dict[str, str]],
                   tenant_id: str, max_tokens: int = 400,
                   temperature: float = 0.4) -> dict[str, Any]:
        self.calls.append({"role": role, "messages": messages,
                           "tenant_id": tenant_id})
        if self.raise_exc is not None:
            raise self.raise_exc
        content = json.dumps(self.payload or {}, ensure_ascii=False)
        return {
            "choices": [{"message": {"content": content}}],
            "aibank_gateway": {
                "role": role, "resolved_model": "mock-fast", "backend": "mock",
                "used_fallback": False, "tenant_id": tenant_id, "latency_ms": 0.5,
            },
        }


@pytest.mark.asyncio
async def test_typical_question_for_individual_entrepreneur() -> None:
    fake = FakeGateway(payload={
        "response": (
            "Для открытия счёта ИП обычно нужен паспорт и ИНН. "
            "После загрузки документов мы запустим автоматическую проверку."
        ),
        "suggested_next_steps": ["Подготовьте паспорт", "Подготовьте ИНН"],
        "require_human_handoff": False,
    })
    req = ChatRequest(
        tenant_id="bank-alpha",
        session_id="s1",
        message="Что нужно для открытия счёта ИП?",
        stage=OnboardingStage.INITIAL,
    )
    result = await handle_chat(req, client=fake)
    assert result.require_human_handoff is False
    assert "паспорт" in result.response.lower()
    assert result.suggested_next_steps
    assert fake.calls[0]["role"] == "ru-chat"
    assert fake.calls[0]["tenant_id"] == "bank-alpha"
    assert result.gateway_metadata["resolved_model"] == "mock-fast"


@pytest.mark.asyncio
async def test_question_about_rejection_reason_escalates() -> None:
    """Клиент спрашивает «почему отказали» — должен escalate без LLM."""
    fake = FakeGateway(payload={
        "response": "Конкретные причины не разглашаются.",
        "suggested_next_steps": [],
        "require_human_handoff": False,
    })
    req = ChatRequest(
        tenant_id="bank-alpha",
        session_id="s2",
        message="Почему отказали в открытии счёта? Скажите конкретную причину!",
        stage=OnboardingStage.VERIFICATION,
    )
    result = await handle_chat(req, client=fake)
    assert result.require_human_handoff is True
    # Hard guard короткозамыкается — до LLM не доходим.
    assert fake.calls == []
    assert result.gateway_metadata.get("short_circuit") == "escalation"


@pytest.mark.asyncio
async def test_off_topic_question_redirects() -> None:
    fake = FakeGateway(payload={"response": "ignored", "suggested_next_steps": [],
                                "require_human_handoff": False})
    req = ChatRequest(
        tenant_id="bank-alpha",
        session_id="s3",
        message="Расскажи анекдот про программистов",
        stage=OnboardingStage.INITIAL,
    )
    result = await handle_chat(req, client=fake)
    assert result.require_human_handoff is False
    assert "онбординг" in result.response.lower() or "счёт" in result.response.lower()
    assert fake.calls == [], "off-topic must short-circuit before LLM"
    assert result.gateway_metadata.get("short_circuit") == "off_topic"


@pytest.mark.asyncio
async def test_complaint_triggers_handoff() -> None:
    fake = FakeGateway(payload={"response": "x", "suggested_next_steps": [],
                                "require_human_handoff": False})
    req = ChatRequest(
        tenant_id="bank-alpha",
        session_id="s4",
        message="У меня жалоба на ваш банк, требую решения!",
        stage=OnboardingStage.VERIFICATION,
    )
    result = await handle_chat(req, client=fake)
    assert result.require_human_handoff is True
    assert fake.calls == []


@pytest.mark.asyncio
async def test_gateway_failure_falls_back_with_handoff() -> None:
    from agent.gateway_client import GatewayError

    fake = FakeGateway(raise_exc=GatewayError("unreachable"))
    req = ChatRequest(
        tenant_id="bank-alpha",
        session_id="s5",
        message="Какие документы нужны для ООО?",
        stage=OnboardingStage.INITIAL,
    )
    result = await handle_chat(req, client=fake)
    assert result.require_human_handoff is True
    assert "недоступен" in result.response.lower()
    assert "llm_error" in result.gateway_metadata


@pytest.mark.asyncio
async def test_role_constant() -> None:
    assert chat_mod.ROLE == "ru-chat"

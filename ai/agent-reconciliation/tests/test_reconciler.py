"""Unit tests for the reconciler with an in-process fake gateway."""
from __future__ import annotations

import json
from typing import Any

import pytest

from agent import reconciler as reconciler_mod
from agent.reconciler import reconcile


class FakeGateway:
    """Protocol-compatible stub for GatewayClient.

    Returns a deterministic chat-completion response shape based on a
    pre-loaded payload (the dict the real LLM would produce in JSON form).
    """

    def __init__(self, payload: dict[str, Any] | str | None = None) -> None:
        self.payload = payload
        self.calls: list[dict[str, Any]] = []

    async def chat(
        self,
        role: str,
        messages: list[dict[str, str]],
        tenant_id: str,
        max_tokens: int = 1024,
        temperature: float = 0.1,
    ) -> dict[str, Any]:
        self.calls.append(
            {"role": role, "messages": messages, "tenant_id": tenant_id}
        )
        if isinstance(self.payload, str):
            content = self.payload
        elif self.payload is None:
            content = json.dumps(
                {"matches": True, "discrepancies": [], "questions_for_client": [],
                 "requires_review": False}
            )
        else:
            content = json.dumps(self.payload, ensure_ascii=False)
        return {
            "choices": [{"message": {"content": content}}],
            "aibank_gateway": {
                "tenant_id": tenant_id,
                "role": role,
                "resolved_model": "mock-fast",
                "backend": "mock",
                "used_fallback": False,
                "latency_ms": 1.0,
            },
        }


@pytest.mark.asyncio
async def test_happy_path_no_discrepancies() -> None:
    extracted = {
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "director": "Иванов Иван Иванович",
        "address": "г. Москва, ул. Ленина, д. 1",
    }
    registry = dict(extracted)

    fake = FakeGateway(
        {"matches": True, "discrepancies": [], "questions_for_client": [],
         "requires_review": False}
    )
    result = await reconcile(
        extracted_data=extracted,
        egrul_data=registry,
        tenant_id="bank-alpha",
        application_id="app-1",
        client=fake,
    )

    assert result.matches is True
    assert result.discrepancies == []
    assert result.requires_review is False
    assert result.gateway_metadata["resolved_model"] == "mock-fast"
    assert fake.calls and fake.calls[0]["role"] == "reconciliation"
    assert fake.calls[0]["tenant_id"] == "bank-alpha"


@pytest.mark.asyncio
async def test_detects_inn_ogrn_director_mismatch() -> None:
    extracted = {
        "inn": "7707083893",
        "ogrn": "1027700132195",
        "director": "Иванов И.И.",
        "address": "Москва",
    }
    registry = {
        "inn": "7707083894",            # diff INN — high
        "ogrn": "1027700999999",         # diff OGRN — high
        "director": "Петров П.П.",       # diff director — high
        "address": "Москва",
    }
    fake = FakeGateway({
        "matches": False,
        "discrepancies": [],
        "questions_for_client": [],
        "requires_review": True,
    })
    result = await reconcile(
        extracted_data=extracted,
        egrul_data=registry,
        tenant_id="bank-alpha",
        application_id="app-2",
        client=fake,
    )

    assert result.matches is False
    fields = {d.field for d in result.discrepancies}
    assert {"inn", "ogrn", "director"}.issubset(fields)
    severities = {d.severity for d in result.discrepancies}
    assert "high" in severities
    assert result.requires_review is True


@pytest.mark.asyncio
async def test_questions_generated_when_llm_silent() -> None:
    """If the LLM returns no questions but discrepancies exist, agent fills fallback."""
    extracted = {"inn": "1", "director": "A"}
    registry = {"inn": "2", "director": "A"}
    fake = FakeGateway({
        "matches": False,
        "discrepancies": [],
        "questions_for_client": [],
        "requires_review": True,
    })
    result = await reconcile(
        extracted_data=extracted,
        egrul_data=registry,
        tenant_id="bank-alpha",
        application_id="app-3",
        client=fake,
    )
    assert result.questions_for_client, "expected fallback questions"
    assert any("inn" in q.lower() or "ИНН" in q for q in result.questions_for_client)


@pytest.mark.asyncio
async def test_uses_llm_questions_when_present() -> None:
    extracted = {"inn": "1"}
    registry = {"inn": "2"}
    fake = FakeGateway({
        "matches": False,
        "discrepancies": [{
            "field": "inn", "extracted_value": "1", "registry_value": "2",
            "severity": "high",
        }],
        "questions_for_client": [
            "Подтвердите, пожалуйста, актуальный ИНН организации.",
        ],
        "requires_review": True,
    })
    result = await reconcile(
        extracted_data=extracted,
        egrul_data=registry,
        tenant_id="bank-alpha",
        application_id="app-4",
        client=fake,
    )
    assert "Подтвердите" in result.questions_for_client[0]


@pytest.mark.asyncio
async def test_gateway_failure_falls_back_to_heuristic(monkeypatch: pytest.MonkeyPatch) -> None:
    """If gateway raises, agent still returns a structured result."""

    class BrokenGateway:
        async def chat(self, *_a: Any, **_kw: Any) -> dict[str, Any]:
            from agent.gateway_client import GatewayError
            raise GatewayError("unreachable")

    extracted = {"inn": "1", "director": "A"}
    registry = {"inn": "2", "director": "B"}
    result = await reconcile(
        extracted_data=extracted,
        egrul_data=registry,
        tenant_id="bank-alpha",
        application_id="app-5",
        client=BrokenGateway(),  # type: ignore[arg-type]
    )
    # Heuristic still detects mismatches.
    assert result.matches is False
    assert {d.field for d in result.discrepancies} == {"inn", "director"}
    assert result.requires_review is True
    assert "llm_error" in result.gateway_metadata


@pytest.mark.asyncio
async def test_role_constant_matches_gateway_yaml() -> None:
    assert reconciler_mod.ROLE == "reconciliation"

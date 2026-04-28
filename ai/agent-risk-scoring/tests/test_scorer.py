"""Unit tests for risk-scoring + LLM-augmented explanation."""
from __future__ import annotations

import pytest

from agent import scorer as scorer_mod
from agent.gateway_client import GatewayError
from agent.scorer import score_application
from models.schemas import ScoringRequest

from tests.conftest import FakeGateway


CLEAN_REQ = ScoringRequest(
    application_id="app-1",
    tenant_id="bank-alpha",
    inn="7707083893",
    ogrn="1027700132195",
    okved="62.01",
    company_age_years=5,
)

HIGH_RISK_REQ = ScoringRequest(
    application_id="app-2",
    tenant_id="bank-alpha",
    inn="7707083893",
    ogrn="1027700132195",
    okved="64.99",
    company_age_years=0,
)

SANCTIONS_REQ = ScoringRequest(
    application_id="app-3",
    tenant_id="bank-alpha",
    inn="7707083893",
    ogrn="1027700132195",
    okved="62.01",
    company_age_years=10,
    facts={"sanctions_match": True},
)


def test_role_constant_matches_gateway_yaml() -> None:
    assert scorer_mod.ROLE == "text-reasoning"


@pytest.mark.asyncio
async def test_scoring_is_deterministic_clean_company() -> None:
    """Same inputs → same outputs every time. CatBoost-stub does not depend on LLM."""
    fake = FakeGateway(raise_exc=GatewayError("offline"))
    r1 = await score_application(CLEAN_REQ, client=fake)
    r2 = await score_application(CLEAN_REQ, client=fake)
    assert r1.score == r2.score
    assert r1.flags == r2.flags == []
    assert r1.fired_rules == r2.fired_rules == []
    assert r1.recommendation == "approve"
    assert r1.score >= 70


@pytest.mark.asyncio
async def test_scoring_high_risk_okved_and_young_company() -> None:
    fake = FakeGateway(raise_exc=GatewayError("offline"))
    r = await score_application(HIGH_RISK_REQ, client=fake)
    assert "high_risk_okved" in r.flags
    assert "company_too_young" in r.flags
    assert r.score < CLEAN_REQ.company_age_years * 100  # cheap upper bound
    assert r.recommendation in ("manual_review", "reject")


@pytest.mark.asyncio
async def test_sanctions_match_blocks() -> None:
    fake = FakeGateway(raise_exc=GatewayError("offline"))
    r = await score_application(SANCTIONS_REQ, client=fake)
    assert r.blocked is True
    assert r.recommendation == "reject"
    assert "sanctions_match" in r.flags


@pytest.mark.asyncio
async def test_llm_explanation_is_used_when_gateway_available() -> None:
    fake = FakeGateway(
        content=(
            "По итогам автоматического скоринга риск-скор составил 80/100. "
            "Сработавших правил нет, флаги отсутствуют — рекомендуется одобрение."
        )
    )
    result = await score_application(CLEAN_REQ, client=fake)
    assert result.explanation_source == "llm"
    assert result.model_used == "mock-fast"
    assert "одобрение" in result.explanation
    assert fake.calls and fake.calls[0]["role"] == "text-reasoning"
    assert fake.calls[0]["tenant_id"] == "bank-alpha"
    assert result.gateway_metadata["resolved_model"] == "mock-fast"


@pytest.mark.asyncio
async def test_llm_failure_falls_back_to_programmatic_explanation() -> None:
    fake = FakeGateway(raise_exc=GatewayError("upstream timeout"))
    result = await score_application(HIGH_RISK_REQ, client=fake)
    assert result.explanation_source == "rule-based"
    assert result.model_used == "rule-based"
    # Programmatic explanation must mention the score.
    assert f"{result.score}/100" in result.explanation
    assert "llm_error" in result.gateway_metadata
    # Score / flags do NOT depend on LLM availability.
    assert "high_risk_okved" in result.flags
    assert "company_too_young" in result.flags


@pytest.mark.asyncio
async def test_llm_does_not_change_score_or_flags() -> None:
    """Even if LLM produces nonsense, blocked/score/flags come from CatBoost-stub."""
    fake = FakeGateway(content="Полная ерунда от модели, не должна повлиять.")
    result = await score_application(SANCTIONS_REQ, client=fake)
    assert result.blocked is True
    assert result.score <= 5
    assert result.recommendation == "reject"


@pytest.mark.asyncio
async def test_programmatic_explanation_when_no_tenant() -> None:
    """Если tenant_id пуст — LLM не вызывается, explanation programmatic."""
    fake = FakeGateway(content="should-not-be-used")
    req = ScoringRequest(
        application_id="app-4",
        tenant_id="",
        inn="7707083893",
        ogrn="1027700132195",
        okved="62.01",
        company_age_years=5,
    )
    result = await score_application(req, client=fake)
    assert fake.calls == []
    assert result.explanation_source == "rule-based"
    assert result.gateway_metadata.get("llm_skipped") == "missing_tenant"

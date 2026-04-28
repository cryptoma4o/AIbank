"""Risk-scoring: deterministic CatBoost-stub + optional LLM-augmented explanation.

ADR-0011: scoring itself is NEVER an LLM call — это детерминированная
рисковая модель (сейчас — stub, в Phase-1 заменяется на CatBoost-сервис
risk-engine). LLM используется ИСКЛЮЧИТЕЛЬНО для генерации
человекочитаемого пояснения по уже посчитанному скору и сработавшим
правилам.
"""
from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any

from agent.gateway_client import (
    GatewayClient,
    GatewayError,
    extract_content,
    extract_metadata,
)
from agent.prompts import EXPLANATION_PROMPT, SYSTEM_PROMPT
from models.schemas import ScoringRequest, ScoringResponse

log = logging.getLogger(__name__)

ROLE = "text-reasoning"


# ---------- Deterministic scoring (CatBoost-stub) -----------------------------


@dataclass
class ScoreResult:
    score: int
    blocked: bool
    flags: list[str]
    fired_rules: list[str]


# Heuristic: imitate a CatBoost output. Полностью детерминирован — те же
# входы дают тот же скор. Reproducibility важна для regulatory-приёмки
# (ADR-0011).
HIGH_RISK_OKVED_PREFIXES = ("64.99", "92.", "47.99", "96.09")


def _stub_score(req: ScoringRequest) -> ScoreResult:
    score = 80
    flags: list[str] = []
    fired_rules: list[str] = []
    blocked = False

    if not req.inn or len(req.inn) not in (10, 12):
        flags.append("inn_invalid_format")
        fired_rules.append("rule.inn.format")
        score -= 30
    if not req.ogrn or len(req.ogrn) not in (13, 15):
        flags.append("ogrn_invalid_format")
        fired_rules.append("rule.ogrn.format")
        score -= 20

    if any(req.okved.startswith(p) for p in HIGH_RISK_OKVED_PREFIXES):
        flags.append("high_risk_okved")
        fired_rules.append("rule.okved.high_risk")
        score -= 25

    if req.company_age_years < 1:
        flags.append("company_too_young")
        fired_rules.append("rule.company_age.lt_1y")
        score -= 15
    elif req.company_age_years < 3:
        score -= 5

    facts = req.facts or {}
    if facts.get("sanctions_match"):
        flags.append("sanctions_match")
        fired_rules.append("rule.sanctions.exact")
        blocked = True
        score = min(score, 5)

    score = max(0, min(100, score))
    return ScoreResult(
        score=score,
        blocked=blocked,
        flags=flags,
        fired_rules=fired_rules,
    )


def _programmatic_explanation(req: ScoringRequest, sr: ScoreResult) -> str:
    pieces: list[str] = [f"Риск-скор: {sr.score}/100."]
    if sr.blocked:
        pieces.append("Заявка автоматически заблокирована по жёстким правилам.")
    elif sr.score >= 70:
        pieces.append("Уровень риска — низкий, можно одобрять.")
    elif sr.score >= 40:
        pieces.append("Уровень риска — средний, требуется ручная проверка.")
    else:
        pieces.append("Уровень риска — высокий, рекомендуется отказ.")
    if sr.fired_rules:
        pieces.append("Сработавшие правила: " + ", ".join(sr.fired_rules) + ".")
    if sr.flags:
        pieces.append("Флаги: " + ", ".join(sr.flags) + ".")
    return " ".join(pieces)


# ---------- LLM-augmented explanation -----------------------------------------


async def explain_with_llm(
    req: ScoringRequest,
    sr: ScoreResult,
    *,
    client: GatewayClient | None = None,
) -> tuple[str, dict[str, Any]]:
    """Generate Russian-language explanation via llm-gateway role:text-reasoning.

    На вход подаются ТОЛЬКО уже посчитанные числа и список сработавших правил —
    LLM не имеет доступа к raw-данным заявки и не может переопределить решение.
    """
    metadata: dict[str, Any] = {"role": ROLE, "application_id": req.application_id}
    if not req.tenant_id:
        metadata["llm_skipped"] = "missing_tenant"
        return "", metadata

    user_prompt = EXPLANATION_PROMPT.format(
        inn=req.inn,
        okved=req.okved,
        company_age_years=req.company_age_years,
        score=sr.score,
        blocked=sr.blocked,
        fired_rules=", ".join(sr.fired_rules) or "нет",
        flags=", ".join(sr.flags) or "нет",
    )
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": user_prompt},
    ]

    gw = client or GatewayClient()
    try:
        response = await gw.chat(
            role=ROLE,
            messages=messages,
            tenant_id=req.tenant_id,
            max_tokens=350,
            temperature=0.3,
        )
        metadata.update(extract_metadata(response))
        text = extract_content(response).strip()
        return text, metadata
    except GatewayError as exc:
        log.warning(
            "LLM explanation failed for app=%s: %s — falling back to programmatic",
            req.application_id, exc,
        )
        metadata["llm_error"] = str(exc)
        return "", metadata


# ---------- Public API --------------------------------------------------------


def _recommend(sr: ScoreResult) -> str:
    if sr.blocked:
        return "reject"
    if sr.score >= 70:
        return "approve"
    return "manual_review"


async def score_application(
    req: ScoringRequest,
    *,
    client: GatewayClient | None = None,
) -> ScoringResponse:
    sr = _stub_score(req)

    llm_text, metadata = await explain_with_llm(req, sr, client=client)
    if llm_text:
        explanation = llm_text
        explanation_source = "llm"
        model_used = str(metadata.get("resolved_model") or "llm")
    else:
        explanation = _programmatic_explanation(req, sr)
        explanation_source = "rule-based"
        model_used = "rule-based"

    return ScoringResponse(
        application_id=req.application_id,
        score=sr.score,
        blocked=sr.blocked,
        flags=sr.flags,
        fired_rules=sr.fired_rules,
        explanation=explanation,
        explanation_source=explanation_source,
        recommendation=_recommend(sr),
        model_used=model_used,
        gateway_metadata=metadata,
    )

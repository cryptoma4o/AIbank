from __future__ import annotations
import logging
import httpx
from models.schemas import ScoringRequest, ScoringResponse
from agent.prompts import EXPLANATION_PROMPT

log = logging.getLogger(__name__)

RISK_ENGINE_URL = "http://risk-engine:8085"
LLM_GATEWAY_URL = "http://llm-gateway:8100"
DEFAULT_MODEL = "gemma-4"


async def score_application(req: ScoringRequest) -> ScoringResponse:
    # Step 1: get score from rule-based risk-engine
    facts = {
        "inn": req.inn,
        "ogrn": req.ogrn,
        "okved": req.okved,
        "company_age_years": req.company_age_years,
        **req.facts,
    }
    score_data: dict = {}
    async with httpx.AsyncClient(timeout=15.0) as client:
        try:
            resp = await client.post(
                f"{RISK_ENGINE_URL}/v1/score",
                json={"application_id": req.application_id, "tenant_id": req.tenant_id, **facts},
            )
            score_data = resp.json()
        except Exception as exc:
            log.warning("risk-engine unavailable: %s", exc)
            score_data = {"score": 50, "blocked": False, "flags": [], "fired_rules": []}

    score = score_data.get("score", 50)
    blocked = score_data.get("blocked", False)
    flags = score_data.get("flags", [])
    fired_rules = score_data.get("fired_rules", [])

    # Step 2: LLM explanation
    explanation = ""
    model_used = DEFAULT_MODEL
    prompt = EXPLANATION_PROMPT.format(
        inn=req.inn, okved=req.okved, company_age_years=req.company_age_years,
        score=score, blocked=blocked, fired_rules=", ".join(fired_rules) or "нет",
        flags=", ".join(flags) or "нет",
    )
    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            resp = await client.post(
                f"{LLM_GATEWAY_URL}/v1/chat/completions",
                json={"model": DEFAULT_MODEL, "messages": [{"role": "user", "content": prompt}],
                      "temperature": 0.3, "max_tokens": 300},
            )
            resp.raise_for_status()
            explanation = resp.json()["choices"][0]["message"]["content"].strip()
    except Exception as exc:
        log.warning("LLM explanation failed: %s", exc)
        explanation = f"Риск-скор: {score}/100. {'Заявка заблокирована по правилам.' if blocked else 'Автоматических блокировок нет.'}"
        model_used = "stub"

    if blocked:
        recommendation = "reject"
    elif score >= 70:
        recommendation = "approve"
    else:
        recommendation = "manual_review"

    return ScoringResponse(
        application_id=req.application_id,
        score=score,
        blocked=blocked,
        flags=flags,
        fired_rules=fired_rules,
        explanation=explanation,
        recommendation=recommendation,
        model_used=model_used,
    )

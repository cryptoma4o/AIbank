"""Happy-path end-to-end onboarding scenario (no real backend).

Walks an applicant from `draft` to `account_opened` through 9 mocked
service calls. Each step:
  1. Registers a respx route for that service's endpoint.
  2. Performs the httpx call the orchestrator would normally do.
  3. Returns the parsed JSON response (so the runner can shape-check it).
  4. Appends an entry to context['audit_events'] (mimics audit-verifier).

Service base URLs are stubs — respx intercepts them in-process. To swap a
real backend in, only the respx setup needs to change; the step bodies stay
identical.
"""
from __future__ import annotations

from typing import Any

import httpx
import respx

from harness.scenarios import Scenario, Step

# Stub service URLs — respx intercepts everything, so these never resolve.
IDENTITY_URL = "http://identity-service.local"
ORCHESTRATOR_URL = "http://onboarding-orchestrator.local"
DOCUMENT_URL = "http://document-service.local"
AGENT_RECON_URL = "http://agent-reconciliation.local"
AGENT_UBO_URL = "http://agent-ubo-tracing.local"
AGENT_RISK_URL = "http://agent-risk-scoring.local"
ABS_URL = "http://abs-connector.local"
AUDIT_URL = "http://audit-verifier.local"


def _record_audit(ctx: dict[str, Any], action: str, **details: Any) -> None:
    """Side-channel: each step pushes one event so we can verify the audit
    chain at the end. Real system would post to audit-service via outbox."""
    ctx["audit_events"].append({"action": action, **details})


# ---------- Steps -----------------------------------------------------------


async def step_register_applicant(ctx: dict[str, Any]) -> dict[str, Any]:
    """1. Регистрация заявителя через identity-service."""
    payload = {"applicant_id": "appl_001", "session_id": "sess_001", "kyc_level": "basic"}
    with respx.mock(base_url=IDENTITY_URL, assert_all_called=False) as router:
        router.post("/v1/applicants").respond(201, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(f"{IDENTITY_URL}/v1/applicants", json={"phone": "+79991234567"})
            r.raise_for_status()
            data = r.json()
    ctx["applicant_id"] = data["applicant_id"]
    _record_audit(ctx, "applicant.registered", applicant_id=data["applicant_id"])
    return data


async def step_create_application(ctx: dict[str, Any]) -> dict[str, Any]:
    """2. Создание заявки в orchestrator (draft → identifying)."""
    payload = {
        "application_id": "app_001",
        "state": "identifying",
        "applicant_id": ctx["applicant_id"],
    }
    with respx.mock(base_url=ORCHESTRATOR_URL, assert_all_called=False) as router:
        router.post("/v1/applications").respond(201, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{ORCHESTRATOR_URL}/v1/applications",
                json={"applicant_id": ctx["applicant_id"]},
            )
            r.raise_for_status()
            data = r.json()
    ctx["application_id"] = data["application_id"]
    _record_audit(ctx, "application.created", application_id=data["application_id"])
    return data


async def step_upload_documents(ctx: dict[str, Any]) -> dict[str, Any]:
    """3. Загрузка документов в document-service (→ collecting_documents)."""
    payload = {
        "application_id": ctx["application_id"],
        "state": "collecting_documents",
        "documents": [
            {"id": "doc_passport", "type": "passport", "status": "stored"},
            {"id": "doc_charter", "type": "charter", "status": "stored"},
        ],
    }
    with respx.mock(base_url=DOCUMENT_URL, assert_all_called=False) as router:
        router.post("/v1/documents/batch").respond(202, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{DOCUMENT_URL}/v1/documents/batch",
                json={"application_id": ctx["application_id"], "files": ["passport.pdf", "charter.pdf"]},
            )
            r.raise_for_status()
            data = r.json()
    _record_audit(ctx, "documents.uploaded", count=len(data["documents"]))
    return data


async def step_run_reconciliation(ctx: dict[str, Any]) -> dict[str, Any]:
    """4. Сверка данных (agent-reconciliation, → validating)."""
    payload = {
        "application_id": ctx["application_id"],
        "state": "validating",
        "requires_review": False,
        "discrepancies": [],
    }
    with respx.mock(base_url=AGENT_RECON_URL, assert_all_called=False) as router:
        router.post("/v1/reconcile").respond(200, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{AGENT_RECON_URL}/v1/reconcile",
                json={"application_id": ctx["application_id"]},
            )
            r.raise_for_status()
            data = r.json()
    _record_audit(ctx, "reconciliation.completed", discrepancies=len(data["discrepancies"]))
    return data


async def step_trace_ubo(ctx: dict[str, Any]) -> dict[str, Any]:
    """5. Раскрытие УБО (agent-ubo-tracing)."""
    payload = {
        "application_id": ctx["application_id"],
        "ubos": [{"name": "Иванов И.И.", "share_percent": 100.0, "is_pep": False}],
        "chain_complete": True,
    }
    with respx.mock(base_url=AGENT_UBO_URL, assert_all_called=False) as router:
        router.post("/v1/ubo/trace").respond(200, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{AGENT_UBO_URL}/v1/ubo/trace",
                json={"application_id": ctx["application_id"], "inn": "7707083893"},
            )
            r.raise_for_status()
            data = r.json()
    _record_audit(ctx, "ubo.traced", ubo_count=len(data["ubos"]))
    return data


async def step_score_risk(ctx: dict[str, Any]) -> dict[str, Any]:
    """6. Скоринг риска (agent-risk-scoring, → risk_assessing)."""
    payload = {
        "application_id": ctx["application_id"],
        "state": "risk_assessing",
        "score": 12,
        "level": "low",
        "recommendation": "approve",
    }
    with respx.mock(base_url=AGENT_RISK_URL, assert_all_called=False) as router:
        router.post("/v1/risk/score").respond(200, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{AGENT_RISK_URL}/v1/risk/score",
                json={"application_id": ctx["application_id"]},
            )
            r.raise_for_status()
            data = r.json()
    ctx["risk_recommendation"] = data["recommendation"]
    _record_audit(ctx, "risk.scored", score=data["score"], level=data["level"])
    return data


async def step_auto_decision(ctx: dict[str, Any]) -> dict[str, Any]:
    """7. Автоматическое решение (orchestrator: risk_assessing → auto_approved → approved)."""
    payload = {
        "application_id": ctx["application_id"],
        "state": "approved",
        "decision": "auto_approved",
        "previous_state": "auto_approved",
    }
    with respx.mock(base_url=ORCHESTRATOR_URL, assert_all_called=False) as router:
        router.post(f"/v1/applications/{ctx['application_id']}/decide").respond(200, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{ORCHESTRATOR_URL}/v1/applications/{ctx['application_id']}/decide",
                json={"recommendation": ctx["risk_recommendation"]},
            )
            r.raise_for_status()
            data = r.json()
    _record_audit(ctx, "decision.made", decision=data["decision"])
    return data


async def step_open_account(ctx: dict[str, Any]) -> dict[str, Any]:
    """8. Открытие счёта в АБС (abs-connector + cft adapter, → account_opened)."""
    payload = {
        "application_id": ctx["application_id"],
        "state": "account_opened",
        "account_number": "40702810900000000001",
        "abs_provider": "cft",
        "opened_at": "2026-04-27T10:00:00Z",
    }
    with respx.mock(base_url=ABS_URL, assert_all_called=False) as router:
        router.post("/v1/accounts").respond(201, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{ABS_URL}/v1/accounts",
                json={"application_id": ctx["application_id"], "provider": "cft"},
            )
            r.raise_for_status()
            data = r.json()
    ctx["account_number"] = data["account_number"]
    _record_audit(ctx, "account.opened", account_number=data["account_number"])
    return data


async def step_verify_audit_chain(ctx: dict[str, Any]) -> dict[str, Any]:
    """9. Проверка цепочки аудита (audit-verifier).

    Does not change application state — verifies retroactively that every
    significant action was logged with a hash-linked event.
    """
    events_in = ctx["audit_events"]
    payload = {
        "application_id": ctx["application_id"],
        "events_total": len(events_in),
        "chain_valid": True,
        "actions": [e["action"] for e in events_in],
    }
    with respx.mock(base_url=AUDIT_URL, assert_all_called=False) as router:
        router.post("/v1/audit/verify").respond(200, json=payload)
        async with httpx.AsyncClient() as client:
            r = await client.post(
                f"{AUDIT_URL}/v1/audit/verify",
                json={"application_id": ctx["application_id"], "events": events_in},
            )
            r.raise_for_status()
            data = r.json()
    return data


# ---------- Scenario assembly -----------------------------------------------

HAPPY_PATH_SCENARIO = Scenario(
    id="happy_path_v1",
    description="Полный путь от регистрации до открытия счёта без ручной обработки.",
    expected_final_state="account_opened",
    steps=[
        Step(
            name="Регистрация заявителя",
            fn=step_register_applicant,
            expected_keys=["applicant_id", "session_id"],
        ),
        Step(
            name="Создание заявки",
            fn=step_create_application,
            expected_state="identifying",
            expected_keys=["application_id", "state"],
        ),
        Step(
            name="Загрузка документов",
            fn=step_upload_documents,
            expected_state="collecting_documents",
            expected_keys=["application_id", "documents"],
        ),
        Step(
            name="Сверка данных",
            fn=step_run_reconciliation,
            expected_state="validating",
            expected_keys=["application_id", "requires_review"],
        ),
        Step(
            name="Раскрытие УБО",
            fn=step_trace_ubo,
            expected_keys=["ubos", "chain_complete"],
        ),
        Step(
            name="Скоринг риска",
            fn=step_score_risk,
            expected_state="risk_assessing",
            expected_keys=["score", "level", "recommendation"],
        ),
        Step(
            name="Автоматическое решение",
            fn=step_auto_decision,
            expected_state="approved",
            expected_keys=["decision", "state"],
        ),
        Step(
            name="Открытие счёта в АБС",
            fn=step_open_account,
            expected_state="account_opened",
            expected_keys=["account_number", "abs_provider"],
        ),
        Step(
            name="Проверка цепочки аудита",
            fn=step_verify_audit_chain,
            expected_keys=["chain_valid", "events_total", "actions"],
        ),
    ],
)

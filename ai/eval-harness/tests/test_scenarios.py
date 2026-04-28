"""Tests for end-to-end synthetic onboarding scenarios.

These run entirely in-process: respx intercepts every httpx call, so no
real services are required. Hard timing budget: 5s for the whole suite.
"""
from __future__ import annotations

import httpx
import pytest
import respx

from harness.scenarios import Scenario, ScenarioRunner, Step
from scenarios import HAPPY_PATH_SCENARIO
from scenarios.happy_path import (
    DOCUMENT_URL,
    HAPPY_PATH_SCENARIO as DIRECT_REF,  # noqa: F401 — sanity import
    step_register_applicant,
    step_create_application,
)


@pytest.mark.asyncio
async def test_happy_path_completes_to_account_opened():
    """The full 9-step happy path must finish in `account_opened` with
    chain_valid=True and an audit event for each significant action."""
    runner = ScenarioRunner()
    result = await runner.run(HAPPY_PATH_SCENARIO)

    assert result.passed, f"scenario failed: {result.error}; results={result.step_results}"
    assert result.final_state == HAPPY_PATH_SCENARIO.expected_final_state == "account_opened"
    assert result.steps_passed == len(HAPPY_PATH_SCENARIO.steps) == 9
    assert result.steps_failed == 0

    # Audit chain: every step that mutated state should have produced an event.
    assert len(result.audit_events) >= 7
    actions = {e["action"] for e in result.audit_events}
    assert {
        "applicant.registered",
        "application.created",
        "documents.uploaded",
        "reconciliation.completed",
        "ubo.traced",
        "risk.scored",
        "decision.made",
        "account.opened",
    }.issubset(actions)


@pytest.mark.asyncio
async def test_happy_path_steps_in_correct_order():
    """State transitions must respect the orchestrator's state machine."""
    runner = ScenarioRunner()
    result = await runner.run(HAPPY_PATH_SCENARIO)

    assert result.passed
    # Only steps with `expected_state` are recorded; others (registration,
    # UBO, audit verification) do not advance state.
    assert result.state_history == [
        "identifying",
        "collecting_documents",
        "validating",
        "risk_assessing",
        "approved",
        "account_opened",
    ]
    # Each consecutive pair must be a valid forward transition (no back-edges).
    # We don't import the Go state map here — just check monotonic progression
    # by hand-validating the canonical sequence.
    expected_order = [
        "identifying",
        "collecting_documents",
        "validating",
        "risk_assessing",
        "approved",
        "account_opened",
    ]
    assert result.state_history == expected_order


@pytest.mark.asyncio
async def test_scenario_fails_fast_on_step_error():
    """Inject a 503 on step 3 (document-service): scenario must stop with
    steps_failed >= 1, final state stuck at the prior transition, and
    later steps NOT executed."""

    async def failing_upload(ctx):
        # Register a 503 instead of a 202.
        with respx.mock(base_url=DOCUMENT_URL, assert_all_called=False) as router:
            router.post("/v1/documents/batch").respond(503, json={"error": "upstream down"})
            async with httpx.AsyncClient() as client:
                r = await client.post(
                    f"{DOCUMENT_URL}/v1/documents/batch",
                    json={"application_id": ctx["application_id"]},
                )
                r.raise_for_status()  # raises HTTPStatusError
                return r.json()

    broken_scenario = Scenario(
        id="happy_path_with_failure",
        description="Synthetic failure at step 3.",
        expected_final_state="account_opened",
        steps=[
            HAPPY_PATH_SCENARIO.steps[0],  # register
            HAPPY_PATH_SCENARIO.steps[1],  # create app
            Step(
                name="Загрузка документов (FAIL)",
                fn=failing_upload,
                expected_state="collecting_documents",
                expected_keys=["application_id"],
            ),
            HAPPY_PATH_SCENARIO.steps[3],  # would be reconciliation — must NOT run
            HAPPY_PATH_SCENARIO.steps[7],  # would be account opening — must NOT run
        ],
    )

    runner = ScenarioRunner(fail_fast=True)
    result = await runner.run(broken_scenario)

    assert not result.passed
    assert result.steps_failed >= 1
    # Step 3 reached, step 4+ should NOT have executed.
    assert len(result.step_results) == 3
    assert result.step_results[0].status == "pass"  # register
    assert result.step_results[1].status == "pass"  # create app
    assert result.step_results[2].status in {"error", "fail"}  # the broken upload
    # Final recorded state is the last successful transition (`identifying`),
    # not `collecting_documents` (which we never reached).
    assert result.final_state == "identifying"
    assert result.error is not None
    assert "Загрузка документов" in result.error


@pytest.mark.asyncio
async def test_scenario_under_5_seconds():
    """Hard CI budget: full mocked happy path must run in < 5s."""
    runner = ScenarioRunner()
    result = await runner.run(HAPPY_PATH_SCENARIO)

    assert result.passed
    assert result.total_duration_ms < 5000.0, (
        f"scenario exceeded 5s budget: {result.total_duration_ms:.0f}ms"
    )
    # Sanity: each step's individual duration is also small (<1s).
    for sr in result.step_results:
        assert sr.duration_ms < 1000.0, f"step {sr.name!r} took {sr.duration_ms:.0f}ms"


@pytest.mark.asyncio
async def test_individual_step_smoke():
    """Smoke-check the first two steps in isolation (helps localise regressions
    when the full scenario fails)."""
    ctx: dict = {"audit_events": []}
    r1 = await step_register_applicant(ctx)
    assert r1["applicant_id"] == "appl_001"
    assert ctx["applicant_id"] == "appl_001"

    r2 = await step_create_application(ctx)
    assert r2["application_id"] == "app_001"
    assert r2["state"] == "identifying"

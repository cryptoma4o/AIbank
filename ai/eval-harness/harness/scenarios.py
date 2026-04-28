"""End-to-end scenario runner for AIbank onboarding flow.

A `Scenario` is an ordered list of `Step`s, each one a small async callable that
exercises one wired service (with a respx-stubbed HTTP backend). Unlike the
single-call `AgentRunner`, the `ScenarioRunner` chains steps, threads context
between them, and stops on first failure.

Cf. docs/domain-model.md § 2.2 — the 15-state machine in
services/onboarding-orchestrator/internal/domain/state.go.
"""
from __future__ import annotations

import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any

# Step receives a mutable context dict (carries application_id, applicant_id, etc.
# between steps) and returns the freshly-produced response payload. Returning the
# raw payload (instead of mutating context only) lets us assert per-step shape
# in addition to chained state.
StepFn = Callable[[dict[str, Any]], Awaitable[dict[str, Any]]]


class StepStatus(StrEnum):
    PASS = "pass"
    FAIL = "fail"
    ERROR = "error"
    SKIP = "skip"


@dataclass
class Step:
    """One node in a scenario.

    Attributes:
        name: human-readable label (Russian for domain-relevant steps).
        fn: async callable, takes shared context, returns response dict.
        expected_state: ApplicationState the orchestrator should be in
            *after* this step succeeds. None if the step does not advance
            state (e.g. audit chain verification).
        expected_keys: minimal shape check on the response payload. Empty
            list means "any non-None response is fine".
    """
    name: str
    fn: StepFn
    expected_state: str | None = None
    expected_keys: list[str] = field(default_factory=list)


@dataclass
class StepResult:
    name: str
    status: StepStatus
    duration_ms: float
    response: dict[str, Any] = field(default_factory=dict)
    state_after: str | None = None
    error: str | None = None


@dataclass
class Scenario:
    """An ordered set of Steps with a final expected state."""
    id: str
    description: str
    steps: list[Step]
    expected_final_state: str
    # Hard timing budget for CI. Default 5s — see test_scenario_under_5_seconds.
    budget_ms: float = 5000.0


@dataclass
class ScenarioResult:
    scenario_id: str
    total_duration_ms: float
    steps_passed: int
    steps_failed: int
    final_state: str | None
    state_history: list[str] = field(default_factory=list)
    step_results: list[StepResult] = field(default_factory=list)
    audit_events: list[dict[str, Any]] = field(default_factory=list)
    error: str | None = None

    @property
    def passed(self) -> bool:
        return (
            self.steps_failed == 0
            and self.error is None
            and self.steps_passed == len(self.step_results)
        )


class ScenarioRunner:
    """Executes a Scenario step-by-step, fail-fast.

    The runner does NOT make real HTTP calls — each step is responsible for
    setting up its own respx route. Steps share a `context` dict so step N can
    pass `application_id` to step N+1.
    """

    def __init__(self, *, fail_fast: bool = True):
        self.fail_fast = fail_fast

    async def run(self, scenario: Scenario) -> ScenarioResult:
        context: dict[str, Any] = {
            "audit_events": [],  # collected as a side-channel by each step
        }
        step_results: list[StepResult] = []
        state_history: list[str] = []
        passed = 0
        failed = 0
        scenario_error: str | None = None
        final_state: str | None = None

        scenario_start = time.monotonic()

        for step in scenario.steps:
            step_start = time.monotonic()
            try:
                response = await step.fn(context)
                duration_ms = (time.monotonic() - step_start) * 1000

                # Shape check.
                missing = [k for k in step.expected_keys if k not in response]
                if missing:
                    failed += 1
                    step_results.append(
                        StepResult(
                            name=step.name,
                            status=StepStatus.FAIL,
                            duration_ms=duration_ms,
                            response=response,
                            error=f"missing expected keys: {missing}",
                        )
                    )
                    if self.fail_fast:
                        scenario_error = f"step '{step.name}' failed shape check"
                        break
                    continue

                # State advance check.
                if step.expected_state is not None:
                    final_state = step.expected_state
                    state_history.append(step.expected_state)

                step_results.append(
                    StepResult(
                        name=step.name,
                        status=StepStatus.PASS,
                        duration_ms=duration_ms,
                        response=response,
                        state_after=step.expected_state,
                    )
                )
                passed += 1

            except Exception as e:  # noqa: BLE001 — runner is the boundary
                duration_ms = (time.monotonic() - step_start) * 1000
                failed += 1
                step_results.append(
                    StepResult(
                        name=step.name,
                        status=StepStatus.ERROR,
                        duration_ms=duration_ms,
                        error=f"{type(e).__name__}: {e}",
                    )
                )
                if self.fail_fast:
                    scenario_error = f"step '{step.name}' raised: {e}"
                    break

        total_ms = (time.monotonic() - scenario_start) * 1000

        return ScenarioResult(
            scenario_id=scenario.id,
            total_duration_ms=total_ms,
            steps_passed=passed,
            steps_failed=failed,
            final_state=final_state,
            state_history=state_history,
            step_results=step_results,
            audit_events=list(context.get("audit_events", [])),
            error=scenario_error,
        )

"""BatchRunner: loads a dataset directory, runs each case through an agent, collects results."""

from __future__ import annotations

import json
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable


@dataclass
class CaseResult:
    case_id: str
    input: Any
    expected_output: Any
    actual_output: Any
    latency_ms: float
    error: str | None = None

    @property
    def success(self) -> bool:
        return self.error is None


@dataclass
class RunReport:
    dataset_dir: str
    total: int = 0
    succeeded: int = 0
    failed: int = 0
    results: list[CaseResult] = field(default_factory=list)

    @property
    def success_rate(self) -> float:
        if self.total == 0:
            return 0.0
        return self.succeeded / self.total

    def summary(self) -> dict[str, Any]:
        return {
            "dataset_dir": self.dataset_dir,
            "total": self.total,
            "succeeded": self.succeeded,
            "failed": self.failed,
            "success_rate": round(self.success_rate, 4),
        }


# Stub agent type: takes the case input dict, returns output dict.
AgentFn = Callable[[dict[str, Any]], dict[str, Any]]


def _stub_agent(input_data: dict[str, Any]) -> dict[str, Any]:
    """Default no-op agent stub. Replace with real agent call."""
    return {}


class BatchRunner:
    """
    Loads all .json case files from a dataset directory and runs each through an agent function.

    Each case file must contain a JSON object with at least:
      - "input": the data passed to the agent
      - "expected_output": the ground truth for metric computation

    Optional per-case field:
      - "id": human-readable identifier (defaults to filename stem)

    Usage::

        runner = BatchRunner(
            dataset_dir="ai/eval-harness/datasets/docs-parsing",
            agent_fn=my_parsing_agent,
        )
        report = runner.run()
        print(report.summary())
    """

    def __init__(
        self,
        dataset_dir: str | Path,
        agent_fn: AgentFn = _stub_agent,
        max_cases: int | None = None,
        file_glob: str = "*.json",
    ) -> None:
        self.dataset_dir = Path(dataset_dir)
        self.agent_fn = agent_fn
        self.max_cases = max_cases
        self.file_glob = file_glob

    def _load_cases(self) -> list[dict[str, Any]]:
        case_files = sorted(self.dataset_dir.glob(self.file_glob))
        if self.max_cases is not None:
            case_files = case_files[: self.max_cases]
        cases = []
        for path in case_files:
            try:
                data = json.loads(path.read_text(encoding="utf-8"))
                if "id" not in data:
                    data["id"] = path.stem
                cases.append(data)
            except (json.JSONDecodeError, OSError) as exc:
                # Log malformed files but continue
                print(f"[BatchRunner] Skipping {path}: {exc}")
        return cases

    def run(self) -> RunReport:
        cases = self._load_cases()
        report = RunReport(dataset_dir=str(self.dataset_dir), total=len(cases))

        for case in cases:
            case_id = case.get("id", "unknown")
            input_data = case.get("input", {})
            expected = case.get("expected_output", {})

            start = time.perf_counter()
            error: str | None = None
            actual: dict[str, Any] = {}

            try:
                actual = self.agent_fn(input_data)
            except Exception as exc:  # noqa: BLE001
                error = f"{type(exc).__name__}: {exc}"

            latency_ms = (time.perf_counter() - start) * 1000

            result = CaseResult(
                case_id=case_id,
                input=input_data,
                expected_output=expected,
                actual_output=actual,
                latency_ms=round(latency_ms, 2),
                error=error,
            )
            report.results.append(result)
            report.total  # already set above via len(cases)
            if error is None:
                report.succeeded += 1
            else:
                report.failed += 1

        return report

    def run_and_print(self) -> RunReport:
        """Convenience: run and print summary to stdout."""
        report = self.run()
        print(json.dumps(report.summary(), ensure_ascii=False, indent=2))
        for r in report.results:
            status = "OK" if r.success else f"FAIL({r.error})"
            print(f"  [{status}] {r.case_id}  {r.latency_ms:.1f}ms")
        return report

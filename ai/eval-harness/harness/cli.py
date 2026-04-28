import asyncio
import json
from pathlib import Path

import click
from rich.console import Console
from rich.table import Table

from .runner import AgentRunner
from .metrics import field_coverage
from .scenarios import ScenarioRunner
from .types import EvalCase

console = Console()


# Lazy-load scenarios so the harness still works in environments without
# `respx` installed (e.g. minimal CI for unit metrics only).
def _load_scenarios() -> dict:
    try:
        from scenarios import HAPPY_PATH_SCENARIO  # noqa: PLC0415
    except ImportError as e:
        raise click.ClickException(
            f"scenarios package unavailable ({e}). Install dev extras: pip install respx"
        ) from e
    return {"happy_path": HAPPY_PATH_SCENARIO}


@click.group()
def main():
    """AIbank AI agent evaluation harness."""
    pass


@main.group()
def scenario():
    """Run multi-step end-to-end onboarding scenarios."""
    pass


@scenario.command("list")
def scenario_list():
    """List available scenarios."""
    scenarios = _load_scenarios()
    table = Table(title="Available scenarios")
    table.add_column("ID", style="cyan")
    table.add_column("Steps", justify="right")
    table.add_column("Final state", style="green")
    table.add_column("Description")
    for sid, sc in scenarios.items():
        table.add_row(sid, str(len(sc.steps)), sc.expected_final_state, sc.description)
    console.print(table)


@scenario.command("run")
@click.argument("scenario_id")
def scenario_run(scenario_id: str):
    """Run a scenario by id (e.g. happy_path)."""
    scenarios = _load_scenarios()
    if scenario_id not in scenarios:
        raise click.ClickException(f"unknown scenario {scenario_id!r}; available: {list(scenarios)}")
    sc = scenarios[scenario_id]
    runner = ScenarioRunner()
    result = asyncio.run(runner.run(sc))

    table = Table(title=f"Scenario {sc.id} — {result.final_state}")
    table.add_column("#", justify="right")
    table.add_column("Step")
    table.add_column("Status", style="bold")
    table.add_column("State after", style="green")
    table.add_column("Latency (ms)", justify="right")
    table.add_column("Error")
    for idx, sr in enumerate(result.step_results, 1):
        color = "green" if sr.status == "pass" else "red"
        table.add_row(
            str(idx),
            sr.name,
            f"[{color}]{sr.status}[/{color}]",
            sr.state_after or "-",
            f"{sr.duration_ms:.0f}",
            sr.error or "",
        )
    console.print(table)
    console.print(f"\nTotal: {result.total_duration_ms:.0f}ms; "
                  f"passed {result.steps_passed}/{len(sc.steps)}; "
                  f"audit events: {len(result.audit_events)}")
    if not result.passed:
        raise SystemExit(1)


@main.command()
@click.argument("cases_file", type=click.Path(exists=True))
@click.option("--agent-url", required=True, help="Agent base URL (e.g. http://localhost:8101)")
@click.option("--fail-threshold", default=0.8, help="Min pass rate to succeed (default: 0.8)")
@click.option("--output", default=None, help="Save JSON report to file")
def run(cases_file: str, agent_url: str, fail_threshold: float, output: str | None):
    """Run evaluation cases against an agent."""
    data = json.loads(Path(cases_file).read_text())
    cases = [EvalCase(**c) for c in data]

    runner = AgentRunner(agent_url)
    report = asyncio.run(runner.run_suite(cases, field_coverage))

    table = Table(title=f"Eval Report — {report.agent} [{report.run_id}]")
    table.add_column("Case ID", style="cyan")
    table.add_column("Status", style="bold")
    table.add_column("Score", justify="right")
    table.add_column("Latency (ms)", justify="right")
    table.add_column("Error")

    for r in report.results:
        color = "green" if r.status == "pass" else ("red" if r.status == "fail" else "yellow")
        table.add_row(
            r.case_id,
            f"[{color}]{r.status}[/{color}]",
            f"{r.score:.2f}",
            f"{r.latency_ms:.0f}",
            r.error or "",
        )

    console.print(table)
    console.print(f"\nPass rate: {report.pass_rate:.1%} ({report.passed}/{report.total})")
    console.print(f"Avg latency: {report.avg_latency_ms:.0f}ms")

    if output:
        import dataclasses
        Path(output).write_text(json.dumps(dataclasses.asdict(report), ensure_ascii=False, indent=2))
        console.print(f"Report saved to {output}")

    if report.pass_rate < fail_threshold:
        raise SystemExit(1)


@main.command()
@click.argument("agent", type=click.Choice(["document-intake", "risk-scoring", "ubo-tracing", "reconciliation", "conversational"]))
@click.option("--count", default=5)
def generate(agent: str, count: int):
    """Generate skeleton eval cases for an agent."""
    templates = {
        "document-intake": {
            "agent": "document-intake",
            "input": {"document_type": "passport", "document_url": "http://example.com/doc.pdf"},
            "expected": {"document_type": "passport", "confidence": None},
            "tags": ["ocr", "extraction"],
        },
        "risk-scoring": {
            "agent": "risk-scoring",
            "input": {"application_id": "app_TEST", "inn": "7707083893", "legal_form": "ooo"},
            "expected": {"recommendation": "approve", "score": None},
            "tags": ["scoring"],
        },
        "ubo-tracing": {
            "agent": "ubo-tracing",
            "input": {"inn": "7707083893", "application_id": "app_TEST"},
            "expected": {"ubos": None},
            "tags": ["ubo"],
        },
        "reconciliation": {
            "agent": "reconciliation",
            "input": {"application_id": "app_TEST", "inn": "7707083893"},
            "expected": {"requires_review": False},
            "tags": ["reconciliation"],
        },
        "conversational": {
            "agent": "conversational",
            "input": {"session_id": "sess_TEST", "message": "Что нужно для открытия счёта?", "stage": "initial"},
            "expected": {"response": None},
            "tags": ["chat"],
        },
    }
    tmpl = templates[agent]
    cases = []
    for i in range(count):
        case = {**tmpl, "id": f"{agent}-{i+1:03d}", "description": f"Auto-generated case {i+1}"}
        cases.append(case)
    click.echo(json.dumps(cases, ensure_ascii=False, indent=2))

import asyncio
import json
from pathlib import Path

import click
from rich.console import Console
from rich.table import Table

from .runner import AgentRunner
from .metrics import field_coverage
from .types import EvalCase

console = Console()


@click.group()
def main():
    """AIbank AI agent evaluation harness."""
    pass


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

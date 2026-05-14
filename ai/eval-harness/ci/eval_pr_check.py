#!/usr/bin/env python3
"""CI script: run all eval-harness corpora and block PRs whose primary metric
drops more than ``--threshold`` percentage points vs the recorded baseline.

Usage::

    cd ai/eval-harness
    LLM_GATEWAY_FORCE_MOCK=1 python ci/eval_pr_check.py \\
        --baseline-file baselines/mock-2026-05-12.json

Behaviour:
* Loads the baseline JSON written by :mod:`ci.compute_baseline`.
* Re-runs every corpus (``docs-parsing``, ``reconciliation``,
  ``ubo-graphs``, ``ru-banking-chat``, ``rag-quality``, ``adversarial``)
  against the *current* code, via the mock agents.
* Compares per-corpus primary metric (F1 or accuracy) against the
  baseline. Exits with status ``1`` if any corpus regressed more than
  ``threshold`` (default: 5 percentage points = 0.05).
* Prints a Markdown table — suitable for posting verbatim as a PR comment.

Per ADR-0011 the regression gate is wired into ``make eval-agents`` and
runs on every PR that touches ``ai/``.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from dataclasses import dataclass
from pathlib import Path

# Add the eval-harness root to sys.path so the top-level packages
# (``harness``, ``metrics``, ``runners``) resolve without an editable install.
_HARNESS_ROOT = Path(__file__).resolve().parents[1]
if str(_HARNESS_ROOT) not in sys.path:
    sys.path.insert(0, str(_HARNESS_ROOT))

from harness.baseline import (  # noqa: E402
    CORPUS_RUNNERS,
    primary_metric_from_section,
    run_all_corpora,
)


@dataclass(frozen=True)
class CorpusDelta:
    name: str
    metric: str
    baseline: float
    current: float
    case_count: int

    @property
    def delta_pp(self) -> float:
        """Difference in percentage points (current minus baseline)."""
        return round((self.current - self.baseline) * 100.0, 2)

    def regressed(self, threshold_pp: float) -> bool:
        """True if the metric dropped by more than ``threshold_pp`` p.p."""
        return self.delta_pp < -threshold_pp


# ---------------------------------------------------------------------------


def _load_baseline(path: Path) -> dict[str, dict]:
    if not path.exists():
        raise FileNotFoundError(f"baseline file not found: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    datasets = data.get("datasets")
    if not isinstance(datasets, dict) or not datasets:
        raise ValueError(f"baseline {path} has no 'datasets' section")
    # Validate metrics are non-null for every corpus (catches the old
    # placeholder file that the gate must NOT accept).
    for name, section in datasets.items():
        metric_name, value = primary_metric_from_section(section)
        if metric_name == "<none>" or value is None:
            raise ValueError(
                f"baseline {path} corpus {name!r} has no usable primary metric "
                "(probably a placeholder with null values)"
            )
    return datasets


def compute_deltas(
    baseline_datasets: dict[str, dict],
    *,
    datasets_root: Path,
) -> list[CorpusDelta]:
    """Run every corpus and return the per-corpus delta vs baseline."""
    current = run_all_corpora(datasets_root)
    deltas: list[CorpusDelta] = []
    for name in CORPUS_RUNNERS:
        if name not in baseline_datasets:
            # Baseline missing a corpus the runner produced — treat as a
            # regression so it cannot be silently ignored.
            cm = current[name]
            deltas.append(
                CorpusDelta(
                    name=name,
                    metric=cm.primary_metric_name,
                    baseline=float("nan"),
                    current=cm.primary_metric_value,
                    case_count=cm.case_count,
                )
            )
            continue
        section = baseline_datasets[name]
        metric_name, baseline_value = primary_metric_from_section(section)
        cm = current[name]
        deltas.append(
            CorpusDelta(
                name=name,
                metric=metric_name if metric_name != "<none>" else cm.primary_metric_name,
                baseline=float(baseline_value),
                current=cm.primary_metric_value,
                case_count=cm.case_count,
            )
        )
    return deltas


def render_markdown_table(
    deltas: list[CorpusDelta],
    *,
    threshold_pp: float,
) -> str:
    lines = [
        "| Corpus | Metric | Baseline | Current | Δ (p.p.) | Status |",
        "|---|---|---:|---:|---:|:---:|",
    ]
    for d in deltas:
        if d.baseline != d.baseline:  # NaN
            status = "NEW"
            baseline_cell = "—"
            delta_cell = "—"
        elif d.regressed(threshold_pp):
            status = "FAIL"
            baseline_cell = f"{d.baseline:.4f}"
            delta_cell = f"{d.delta_pp:+.2f}"
        else:
            status = "OK"
            baseline_cell = f"{d.baseline:.4f}"
            delta_cell = f"{d.delta_pp:+.2f}"
        lines.append(
            f"| {d.name} ({d.case_count}) | {d.metric} | {baseline_cell} "
            f"| {d.current:.4f} | {delta_cell} | {status} |"
        )
    return "\n".join(lines)


# ---------------------------------------------------------------------------


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Eval PR regression check")
    parser.add_argument(
        "--baseline-file",
        default=str(_HARNESS_ROOT / "baselines" / "mock-2026-05-12.json"),
        help="Path to baseline metrics JSON",
    )
    parser.add_argument(
        "--datasets",
        default=str(_HARNESS_ROOT / "datasets"),
        help="Root directory containing the per-corpus dataset folders",
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=0.05,
        help="Maximum allowed primary-metric regression as a fraction "
        "(default: 0.05 = 5 percentage points, per ADR-0011)",
    )
    parser.add_argument(
        "--output-markdown",
        default=None,
        help="If set, write the per-corpus comparison Markdown table to this file",
    )
    parser.add_argument(
        "--allow-non-mock",
        action="store_true",
        help="Bypass the LLM_GATEWAY_FORCE_MOCK=1 guard (NOT recommended in CI)",
    )
    args = parser.parse_args(argv)

    if not args.allow_non_mock:
        os.environ.setdefault("LLM_GATEWAY_FORCE_MOCK", "1")

    baseline_path = Path(args.baseline_file)
    try:
        baseline_datasets = _load_baseline(baseline_path)
    except (FileNotFoundError, ValueError) as exc:
        print(f"[eval_pr_check] ERROR: {exc}", file=sys.stderr)
        return 2

    datasets_root = Path(args.datasets)
    if not datasets_root.is_dir():
        print(
            f"[eval_pr_check] ERROR: datasets root not found: {datasets_root}",
            file=sys.stderr,
        )
        return 2

    deltas = compute_deltas(baseline_datasets, datasets_root=datasets_root)
    threshold_pp = args.threshold * 100.0

    table = render_markdown_table(deltas, threshold_pp=threshold_pp)
    print(f"### Eval-harness regression check (threshold: {threshold_pp:.1f} p.p.)")
    print()
    print(table)
    print()

    if args.output_markdown:
        Path(args.output_markdown).write_text(table + "\n", encoding="utf-8")

    regressions = [d for d in deltas if d.baseline == d.baseline and d.regressed(threshold_pp)]
    if regressions:
        print(
            f"[eval_pr_check] FAIL: {len(regressions)} corpus regression(s) "
            f"> {threshold_pp:.1f} p.p.: " + ", ".join(r.name for r in regressions),
            file=sys.stderr,
        )
        return 1

    print("[eval_pr_check] PASS: all corpora within tolerance.")
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())

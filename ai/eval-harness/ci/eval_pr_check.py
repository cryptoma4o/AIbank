#!/usr/bin/env python3
"""CI script: runs a fast eval subset (~50 cases) and exits 1 if F1 regression > 5%.

Usage:
    python ai/eval-harness/ci/eval_pr_check.py \\
        --dataset-dir ai/eval-harness/datasets/docs-parsing \\
        --baseline-file ai/eval-harness/ci/baseline.json \\
        --max-cases 50 \\
        --threshold 0.05

If no baseline exists, the script writes one and exits 0.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# Allow running from repo root without installing the package.
_REPO_ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(_REPO_ROOT))

from ai.eval_harness.metrics.document_metrics import compute_dataset_f1  # noqa: E402
from ai.eval_harness.runners.batch_runner import BatchRunner  # noqa: E402


def _stub_agent(input_data: dict) -> dict:
    """Replace with a real agent import in production."""
    return {}


def _extract_fields(results) -> tuple[list[dict], list[dict]]:
    predictions = []
    ground_truths = []
    for r in results:
        predictions.append(r.actual_output if isinstance(r.actual_output, dict) else {})
        ground_truths.append(
            r.expected_output if isinstance(r.expected_output, dict) else {}
        )
    return predictions, ground_truths


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Eval PR regression check")
    parser.add_argument(
        "--dataset-dir",
        required=True,
        help="Path to dataset directory containing .json case files",
    )
    parser.add_argument(
        "--baseline-file",
        default=str(Path(__file__).parent / "baseline.json"),
        help="Path to baseline metrics JSON (created if missing)",
    )
    parser.add_argument(
        "--max-cases",
        type=int,
        default=50,
        help="Maximum number of cases to evaluate (default: 50)",
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=0.05,
        help="Maximum allowed F1 regression as a fraction (default: 0.05 = 5%%)",
    )
    args = parser.parse_args(argv)

    dataset_dir = Path(args.dataset_dir)
    baseline_path = Path(args.baseline_file)

    if not dataset_dir.exists():
        print(f"[eval_pr_check] ERROR: dataset dir not found: {dataset_dir}")
        return 1

    runner = BatchRunner(
        dataset_dir=dataset_dir,
        agent_fn=_stub_agent,
        max_cases=args.max_cases,
    )
    report = runner.run()

    if report.total == 0:
        print(f"[eval_pr_check] No cases found in {dataset_dir}. Skipping check.")
        return 0

    predictions, ground_truths = _extract_fields(report.results)
    current_metrics = compute_dataset_f1(predictions, ground_truths)
    current_f1: float = current_metrics["macro_f1"]

    print(f"[eval_pr_check] Evaluated {report.total} cases.")
    print(f"[eval_pr_check] Current macro F1: {current_f1:.4f}")

    if not baseline_path.exists():
        baseline_path.parent.mkdir(parents=True, exist_ok=True)
        baseline_path.write_text(
            json.dumps({"macro_f1": current_f1, "metrics": current_metrics}, indent=2),
            encoding="utf-8",
        )
        print(f"[eval_pr_check] No baseline found. Wrote baseline to {baseline_path}.")
        return 0

    baseline = json.loads(baseline_path.read_text(encoding="utf-8"))
    baseline_f1: float = baseline.get("macro_f1", 0.0)

    regression = baseline_f1 - current_f1
    print(f"[eval_pr_check] Baseline macro F1: {baseline_f1:.4f}")
    print(f"[eval_pr_check] Regression: {regression:+.4f} (threshold: -{args.threshold:.4f})")

    if regression > args.threshold:
        print(
            f"[eval_pr_check] FAIL: F1 dropped by {regression:.4f} "
            f"(>{args.threshold:.4f} allowed). Blocking PR."
        )
        return 1

    print("[eval_pr_check] PASS: F1 within acceptable range.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

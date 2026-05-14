#!/usr/bin/env python3
"""Compute a fresh mock baseline by running every corpus through the mock agents.

Run from ``ai/eval-harness/`` so that the ``harness`` / ``metrics`` /
``runners`` top-level packages are importable::

    cd ai/eval-harness
    LLM_GATEWAY_FORCE_MOCK=1 python ci/compute_baseline.py \\
        --output baselines/mock-2026-05-12.json

If the output file already exists, the script overwrites it; pass
``--check`` to compare against a baseline instead of writing one.

Per ADR-0011 the regression gate compares per-corpus primary metrics
against a fixed mock baseline and blocks any PR that drops more than
5 percentage points on any corpus.
"""

from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import sys
from pathlib import Path

# Add the repo's eval-harness root to sys.path so that ``harness.X`` and
# its siblings (``metrics``, ``runners``) resolve without an editable install.
_HARNESS_ROOT = Path(__file__).resolve().parents[1]
if str(_HARNESS_ROOT) not in sys.path:
    sys.path.insert(0, str(_HARNESS_ROOT))

from harness.baseline import (  # noqa: E402
    assert_force_mock_env,
    build_baseline_payload,
    run_all_corpora,
)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Compute mock baseline for eval-harness")
    parser.add_argument(
        "--output",
        required=True,
        help="Destination JSON path (e.g. baselines/mock-2026-05-12.json)",
    )
    parser.add_argument(
        "--datasets",
        default=str(_HARNESS_ROOT / "datasets"),
        help="Root directory containing the per-corpus dataset folders",
    )
    parser.add_argument(
        "--fixed-by",
        default="eval-harness compute_baseline.py (mock-gateway)",
        help="Author label written into the baseline JSON",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Overwrite output if it exists",
    )
    parser.add_argument(
        "--allow-non-mock",
        action="store_true",
        help="Bypass the LLM_GATEWAY_FORCE_MOCK=1 guard (NOT recommended)",
    )
    args = parser.parse_args(argv)

    if not args.allow_non_mock:
        # Default to setting the env var if the caller forgot — this keeps
        # the script useful as a one-shot dev tool without surprising people.
        os.environ.setdefault("LLM_GATEWAY_FORCE_MOCK", "1")
        assert_force_mock_env()

    datasets_root = Path(args.datasets)
    if not datasets_root.is_dir():
        print(f"[compute_baseline] datasets root not found: {datasets_root}", file=sys.stderr)
        return 2

    out_path = Path(args.output)
    if out_path.exists() and not args.force:
        print(
            f"[compute_baseline] {out_path} exists; pass --force to overwrite.",
            file=sys.stderr,
        )
        return 2

    print(f"[compute_baseline] datasets root: {datasets_root}")
    corpora = run_all_corpora(datasets_root)

    print("[compute_baseline] per-corpus primary metrics:")
    for name, cm in corpora.items():
        print(
            f"  - {name:<18s} cases={cm.case_count:<3d} "
            f"{cm.primary_metric_name}={cm.primary_metric_value:.4f}"
        )

    payload = build_baseline_payload(
        corpora,
        fixed_at=_dt.date.today().isoformat(),
        fixed_by=args.fixed_by,
        backend="mock",
    )
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"[compute_baseline] wrote {out_path}")
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())

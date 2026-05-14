"""Tests for the CI regression gate (`ci/eval_pr_check.py`).

Covers three contracts:
1. Missing or placeholder (null-metric) baselines must abort with a non-
   zero exit code BEFORE running the corpora — so a stale placeholder
   can't silently make the gate green.
2. A simulated regression (current metric << baseline) must exit 1.
3. The happy path — current metrics within tolerance — must exit 0 and
   emit the comparison Markdown table.

All tests run fully offline against the bundled fixtures. We never
spawn the agent HTTP services or touch the live llm-gateway: every
corpus uses the in-process mock agents from
:mod:`harness.mock_agents`.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import pytest

# Ensure the eval-harness root is importable as ``harness.X`` / ``ci.X``.
_HARNESS_ROOT = Path(__file__).resolve().parents[1]
if str(_HARNESS_ROOT) not in sys.path:
    sys.path.insert(0, str(_HARNESS_ROOT))

from ci import eval_pr_check  # noqa: E402
from harness.baseline import (  # noqa: E402
    build_baseline_payload,
    run_all_corpora,
)


@pytest.fixture(autouse=True)
def _force_mock_env(monkeypatch):
    """Every test in this module runs as if LLM_GATEWAY_FORCE_MOCK=1."""
    monkeypatch.setenv("LLM_GATEWAY_FORCE_MOCK", "1")


@pytest.fixture(scope="module")
def datasets_root() -> Path:
    return _HARNESS_ROOT / "datasets"


@pytest.fixture(scope="module")
def real_baseline_path() -> Path:
    """The freshly-computed baseline committed to the repo."""
    return _HARNESS_ROOT / "baselines" / "mock-2026-05-12.json"


# ---------------------------------------------------------------------------
# 1. Missing / placeholder baselines must fail loudly
# ---------------------------------------------------------------------------


def test_missing_baseline_returns_error(tmp_path: Path, datasets_root: Path):
    """A non-existent baseline file must abort with exit code 2."""
    missing = tmp_path / "no-such-baseline.json"
    rc = eval_pr_check.main(
        [
            "--baseline-file",
            str(missing),
            "--datasets",
            str(datasets_root),
        ]
    )
    assert rc == 2


def test_placeholder_baseline_with_null_metrics_returns_error(
    tmp_path: Path, datasets_root: Path
):
    """A baseline whose metrics are null (placeholder) must NOT be accepted."""
    placeholder = {
        "schema_version": 1,
        "backend": "mock",
        "fixed_at": "1970-01-01",
        "fixed_by": "test",
        "datasets": {
            "docs-parsing": {
                "case_count": 50,
                "metrics": {
                    "field_coverage_f1": None,
                    "exact_match_rate": None,
                    "avg_latency_ms": None,
                    "p95_latency_ms": None,
                },
            }
        },
    }
    path = tmp_path / "placeholder.json"
    path.write_text(json.dumps(placeholder), encoding="utf-8")

    rc = eval_pr_check.main(
        [
            "--baseline-file",
            str(path),
            "--datasets",
            str(datasets_root),
        ]
    )
    assert rc == 2


def test_real_baseline_loads_with_primary_metric(real_baseline_path: Path):
    """The committed mock-2026-05-12 baseline must have a usable primary metric
    on every corpus — otherwise the CI gate cannot compare against it."""
    datasets = eval_pr_check._load_baseline(real_baseline_path)
    for name, section in datasets.items():
        metric, value = eval_pr_check.primary_metric_from_section(section)
        assert metric != "<none>", f"{name} has no primary metric"
        assert 0.0 <= value <= 1.0, f"{name} primary metric out of [0,1]: {value}"


# ---------------------------------------------------------------------------
# 2. Regression must exit 1
# ---------------------------------------------------------------------------


def _bumped_baseline(datasets_root: Path, *, bump_pp: float) -> dict:
    """Compute the real metrics, then add ``bump_pp`` percentage points to
    each primary metric to simulate a baseline that the CURRENT code can no
    longer match."""
    corpora = run_all_corpora(datasets_root)
    for cm in corpora.values():
        new_value = min(1.0, cm.primary_metric_value + bump_pp / 100.0)
        cm.metrics[cm.primary_metric_name] = new_value
        cm.primary_metric_value = new_value
    return build_baseline_payload(
        corpora,
        fixed_at="2099-01-01",
        fixed_by="test_regression",
        backend="mock",
    )


def test_regression_above_threshold_exits_1(tmp_path: Path, datasets_root: Path):
    """Adding >5 p.p. to every baseline metric => current code regresses; exit 1."""
    payload = _bumped_baseline(datasets_root, bump_pp=10.0)
    path = tmp_path / "bumped.json"
    path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")

    md_out = tmp_path / "report.md"
    rc = eval_pr_check.main(
        [
            "--baseline-file",
            str(path),
            "--datasets",
            str(datasets_root),
            "--threshold",
            "0.05",
            "--output-markdown",
            str(md_out),
        ]
    )
    assert rc == 1
    # The Markdown report must be produced and contain a FAIL marker for at
    # least one corpus.
    text = md_out.read_text(encoding="utf-8")
    assert "FAIL" in text
    # The header row is present.
    assert "| Corpus |" in text


def test_regression_below_threshold_exits_0(tmp_path: Path, datasets_root: Path):
    """Adding a tiny bump (≤ threshold) must NOT trigger the gate."""
    # +2 p.p. is below the 5 p.p. default threshold.
    payload = _bumped_baseline(datasets_root, bump_pp=2.0)
    path = tmp_path / "tiny_bump.json"
    path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")

    rc = eval_pr_check.main(
        [
            "--baseline-file",
            str(path),
            "--datasets",
            str(datasets_root),
            "--threshold",
            "0.05",
        ]
    )
    assert rc == 0


# ---------------------------------------------------------------------------
# 3. Happy path
# ---------------------------------------------------------------------------


def test_no_regression_exits_0_with_markdown_table(
    tmp_path: Path, datasets_root: Path, real_baseline_path: Path
):
    """Running against the freshly committed baseline must pass."""
    md_out = tmp_path / "report.md"
    rc = eval_pr_check.main(
        [
            "--baseline-file",
            str(real_baseline_path),
            "--datasets",
            str(datasets_root),
            "--output-markdown",
            str(md_out),
        ]
    )
    assert rc == 0
    text = md_out.read_text(encoding="utf-8")
    # The table contains a row for each of the six corpora.
    for corpus in (
        "docs-parsing",
        "reconciliation",
        "ubo-graphs",
        "ru-banking-chat",
        "rag-quality",
        "adversarial",
    ):
        assert corpus in text


# ---------------------------------------------------------------------------
# Determinism: a second baseline run produces identical numbers
# ---------------------------------------------------------------------------


def test_compute_baseline_is_deterministic(datasets_root: Path):
    """Mock-agent metrics must be byte-stable across runs (no randomness)."""
    a = run_all_corpora(datasets_root)
    b = run_all_corpora(datasets_root)
    for name in a:
        assert a[name].primary_metric_value == b[name].primary_metric_value, (
            f"non-deterministic metric for {name}: "
            f"{a[name].primary_metric_value} != {b[name].primary_metric_value}"
        )


# ---------------------------------------------------------------------------
# Guard: tests must not perform any network call
# ---------------------------------------------------------------------------


def test_no_network_during_baseline_run(monkeypatch, datasets_root: Path):
    """Patch httpx.Client.send to fail loud — if anything dials out, the test
    blows up immediately rather than silently succeeding on a flaky connection.
    """
    import httpx

    def _no_network(*args, **kwargs):  # noqa: ANN001
        raise AssertionError("network call attempted during offline baseline run")

    monkeypatch.setattr(httpx.Client, "send", _no_network, raising=False)
    monkeypatch.setattr(httpx.AsyncClient, "send", _no_network, raising=False)
    # Should still produce metrics without going through httpx at all.
    corpora = run_all_corpora(datasets_root)
    assert set(corpora) == {
        "docs-parsing",
        "reconciliation",
        "ubo-graphs",
        "ru-banking-chat",
        "rag-quality",
        "adversarial",
    }


# Keep an import-side smoke test so a syntax error in ci.eval_pr_check
# never sneaks in undetected.
def test_module_imports_are_stable():
    assert hasattr(eval_pr_check, "main")
    assert hasattr(eval_pr_check, "compute_deltas")
    assert hasattr(eval_pr_check, "render_markdown_table")
    # The placeholder _stub_agent must be gone — the gate now uses real
    # corpus runners.
    assert not hasattr(eval_pr_check, "_stub_agent"), (
        "stub agent should be removed; use harness.baseline.run_all_corpora"
    )


# Use `os` import to keep the linter happy when env-poking helpers grow.
_ = os

"""Тесты учёта токенов и расчёта стоимости."""
from __future__ import annotations

from core.usage import UsageTracker


def test_record_first_request_creates_row():
    t = UsageTracker()
    t.record(
        tenant_id="bank-alpha",
        model="gemma-test",
        role="vision",
        prompt_tokens=1000,
        completion_tokens=500,
        cost_per_1k_input_kop=100,
        cost_per_1k_output_kop=200,
    )
    rows = t.snapshot()
    assert len(rows) == 1
    row = rows[0]
    assert row["tenant_id"] == "bank-alpha"
    assert row["model"] == "gemma-test"
    assert row["requests"] == 1
    assert row["prompt_tokens"] == 1000
    assert row["completion_tokens"] == 500
    # 1000/1000 * 100 = 100 kop input + 500/1000 * 200 = 100 kop output = 200 kop total
    assert row["total_kopecks"] == 200


def test_record_aggregates_for_same_key():
    t = UsageTracker()
    for _ in range(3):
        t.record(
            tenant_id="bank-alpha",
            model="gemma-test",
            role="vision",
            prompt_tokens=400,
            completion_tokens=100,
            cost_per_1k_input_kop=100,
            cost_per_1k_output_kop=200,
        )
    rows = t.snapshot()
    assert len(rows) == 1
    assert rows[0]["requests"] == 3
    assert rows[0]["prompt_tokens"] == 1200
    assert rows[0]["completion_tokens"] == 300


def test_filter_by_tenant_isolates():
    t = UsageTracker()
    t.record("bank-alpha", "gemma-test", None, 100, 100, 50, 100)
    t.record("bank-beta",  "gemma-test", None, 200, 200, 50, 100)

    alpha = t.snapshot(tenant_id="bank-alpha")
    beta = t.snapshot(tenant_id="bank-beta")
    assert len(alpha) == 1 and alpha[0]["tenant_id"] == "bank-alpha"
    assert len(beta) == 1 and beta[0]["tenant_id"] == "bank-beta"


def test_aggregate_by_tenant_sums_across_models():
    t = UsageTracker()
    t.record("bank-alpha", "gemma-test", None, 1000, 1000, 100, 200)  # 100 + 200 = 300 kop
    t.record("bank-alpha", "qwen-test",  None, 1000, 1000, 50, 100)   # 50 + 100  = 150 kop

    agg = t.aggregate_by_tenant()
    assert agg["bank-alpha"]["requests"] == 2
    assert agg["bank-alpha"]["prompt_tokens"] == 2000
    assert agg["bank-alpha"]["total_kopecks"] == 450


def test_partial_thousand_floors_cost():
    """500 input tokens at 100 kop/1k = 50 kop; не 50.0, не округление вверх."""
    t = UsageTracker()
    t.record("bank-alpha", "m", None, 500, 0, 100, 0)
    rows = t.snapshot()
    # 500 * 100 // 1000 = 50
    assert rows[0]["total_kopecks"] == 50

    # Под порогом — стоимость 0 (мы используем integer //; это намеренно).
    t.reset()
    t.record("bank-alpha", "m", None, 5, 0, 100, 0)
    rows = t.snapshot()
    assert rows[0]["total_kopecks"] == 0  # 5 * 100 // 1000 = 0

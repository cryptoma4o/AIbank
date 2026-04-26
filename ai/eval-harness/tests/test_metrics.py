import pytest
from harness.metrics import exact_match, field_coverage, score_in_range


def test_exact_match():
    assert exact_match("approve", "approve") == 1.0
    assert exact_match("approve", "reject") == 0.0


def test_field_coverage():
    actual = {"a": 1, "b": 2, "c": None}
    expected = {"a": 1, "b": 2, "c": 3}
    # c is None in actual, so 2/3
    assert field_coverage(actual, expected) == pytest.approx(2/3)


def test_score_in_range():
    assert score_in_range({"score": 80}, "score", 0, 100) == 1.0
    assert score_in_range({"score": 80}, "score", 90, 100) == 0.0
    assert score_in_range({}, "score", 0, 100) == 0.0

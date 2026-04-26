from __future__ import annotations
from typing import Any


def exact_match(actual: Any, expected: Any) -> float:
    """1.0 if exact match, 0.0 otherwise."""
    return 1.0 if actual == expected else 0.0


def field_coverage(actual: dict, expected: dict) -> float:
    """Fraction of expected keys present and non-None in actual."""
    if not expected:
        return 1.0
    matched = sum(1 for k in expected if k in actual and actual[k] is not None)
    return matched / len(expected)


def score_in_range(actual: dict, field: str, min_val: float, max_val: float) -> float:
    """1.0 if actual[field] is within [min_val, max_val]."""
    val = actual.get(field)
    if val is None:
        return 0.0
    try:
        return 1.0 if min_val <= float(val) <= max_val else 0.0
    except (TypeError, ValueError):
        return 0.0


def recommendation_correct(actual: dict, expected: dict) -> float:
    """Check if recommendation matches expected."""
    return exact_match(actual.get("recommendation"), expected.get("recommendation"))

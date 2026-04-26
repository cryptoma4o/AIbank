"""Field-level F1 score calculation for document parsing evaluation.

Each document is represented as a flat dict of field_name -> string value.
Evaluation is done field-by-field; token-level F1 is computed per field
(standard QA-style), then macro-averaged across all fields.
"""

from __future__ import annotations

import re
from collections import Counter
from dataclasses import dataclass


def _normalize(text: str) -> str:
    """Lowercase, collapse whitespace, strip punctuation for token matching."""
    text = text.lower()
    text = re.sub(r"[^\w\s]", " ", text, flags=re.UNICODE)
    return " ".join(text.split())


def _token_counts(text: str) -> Counter:
    return Counter(_normalize(text).split())


def token_f1(predicted: str, ground_truth: str) -> float:
    """Compute token-level F1 between two strings (QA-style)."""
    pred_tokens = _token_counts(predicted)
    gold_tokens = _token_counts(ground_truth)

    common = pred_tokens & gold_tokens
    num_common = sum(common.values())

    if num_common == 0:
        return 0.0

    precision = num_common / sum(pred_tokens.values())
    recall = num_common / sum(gold_tokens.values())
    f1 = 2 * precision * recall / (precision + recall)
    return round(f1, 4)


@dataclass
class FieldF1Result:
    field: str
    predicted: str
    ground_truth: str
    f1: float
    exact_match: bool


@dataclass
class DocumentF1Result:
    fields: list[FieldF1Result]

    @property
    def macro_f1(self) -> float:
        if not self.fields:
            return 0.0
        return round(sum(f.f1 for f in self.fields) / len(self.fields), 4)

    @property
    def exact_match_rate(self) -> float:
        if not self.fields:
            return 0.0
        return round(sum(1 for f in self.fields if f.exact_match) / len(self.fields), 4)

    def per_field_summary(self) -> dict[str, float]:
        return {f.field: f.f1 for f in self.fields}


def compute_document_f1(
    predicted: dict[str, str],
    ground_truth: dict[str, str],
    fields: list[str] | None = None,
) -> DocumentF1Result:
    """
    Compute field-level F1 for a parsed document.

    Args:
        predicted: Agent output dict, field_name -> extracted string value.
        ground_truth: Gold standard dict, field_name -> correct string value.
        fields: Subset of fields to evaluate. If None, uses union of both dicts.

    Returns:
        DocumentF1Result with per-field and macro scores.
    """
    if fields is None:
        fields = sorted(set(predicted) | set(ground_truth))

    field_results: list[FieldF1Result] = []
    for field_name in fields:
        pred_val = str(predicted.get(field_name, ""))
        gold_val = str(ground_truth.get(field_name, ""))
        f1 = token_f1(pred_val, gold_val)
        exact = _normalize(pred_val) == _normalize(gold_val)
        field_results.append(
            FieldF1Result(
                field=field_name,
                predicted=pred_val,
                ground_truth=gold_val,
                f1=f1,
                exact_match=exact,
            )
        )

    return DocumentF1Result(fields=field_results)


def compute_dataset_f1(
    predictions: list[dict[str, str]],
    ground_truths: list[dict[str, str]],
    fields: list[str] | None = None,
) -> dict[str, float]:
    """
    Aggregate macro-F1 across a full dataset.

    Args:
        predictions: List of predicted field dicts (one per document).
        ground_truths: List of gold standard field dicts (same order).
        fields: Fields to evaluate. If None, inferred from first gold sample.

    Returns:
        Dict with 'macro_f1', 'exact_match_rate', and per-field averages.
    """
    if len(predictions) != len(ground_truths):
        raise ValueError(
            f"predictions ({len(predictions)}) and ground_truths ({len(ground_truths)}) "
            "must have the same length"
        )

    if not predictions:
        return {"macro_f1": 0.0, "exact_match_rate": 0.0}

    results = [
        compute_document_f1(pred, gold, fields)
        for pred, gold in zip(predictions, ground_truths)
    ]

    all_fields: list[str] = fields or sorted(
        set().union(*(set(gt) for gt in ground_truths))
    )

    per_field_f1: dict[str, list[float]] = {f: [] for f in all_fields}
    for result in results:
        summary = result.per_field_summary()
        for f in all_fields:
            per_field_f1[f].append(summary.get(f, 0.0))

    aggregated: dict[str, float] = {}
    for f, scores in per_field_f1.items():
        aggregated[f"field/{f}"] = round(sum(scores) / len(scores), 4)

    aggregated["macro_f1"] = round(
        sum(r.macro_f1 for r in results) / len(results), 4
    )
    aggregated["exact_match_rate"] = round(
        sum(r.exact_match_rate for r in results) / len(results), 4
    )
    return aggregated

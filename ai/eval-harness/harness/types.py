from __future__ import annotations
from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any


class EvalStatus(StrEnum):
    PASS = "pass"
    FAIL = "fail"
    ERROR = "error"
    SKIP = "skip"


@dataclass
class EvalCase:
    id: str
    agent: str          # e.g. "document-intake", "risk-scoring"
    input: dict[str, Any]
    expected: dict[str, Any]
    tags: list[str] = field(default_factory=list)
    description: str = ""


@dataclass
class EvalResult:
    case_id: str
    agent: str
    status: EvalStatus
    actual: dict[str, Any]
    score: float        # 0.0 – 1.0
    latency_ms: float
    error: str | None = None
    details: dict[str, Any] = field(default_factory=dict)


@dataclass
class EvalReport:
    run_id: str
    agent: str
    total: int
    passed: int
    failed: int
    errors: int
    skipped: int
    pass_rate: float
    avg_latency_ms: float
    results: list[EvalResult]
    timestamp: str

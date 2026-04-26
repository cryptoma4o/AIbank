from __future__ import annotations
import asyncio
import time
import uuid
from datetime import datetime, timezone

import httpx

from .types import EvalCase, EvalResult, EvalReport, EvalStatus


class AgentRunner:
    def __init__(self, base_url: str, timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    async def run_case(self, case: EvalCase, score_fn) -> EvalResult:
        start = time.monotonic()
        try:
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                resp = await client.post(f"{self.base_url}/v1/process", json=case.input)
                resp.raise_for_status()
                actual = resp.json()
            latency_ms = (time.monotonic() - start) * 1000
            score = score_fn(actual, case.expected)
            status = EvalStatus.PASS if score >= 0.8 else EvalStatus.FAIL
            return EvalResult(
                case_id=case.id,
                agent=case.agent,
                status=status,
                actual=actual,
                score=score,
                latency_ms=latency_ms,
            )
        except Exception as e:
            latency_ms = (time.monotonic() - start) * 1000
            return EvalResult(
                case_id=case.id,
                agent=case.agent,
                status=EvalStatus.ERROR,
                actual={},
                score=0.0,
                latency_ms=latency_ms,
                error=str(e),
            )

    async def run_suite(self, cases: list[EvalCase], score_fn) -> EvalReport:
        run_id = str(uuid.uuid4())[:8]
        results = await asyncio.gather(*[self.run_case(c, score_fn) for c in cases])
        passed = sum(1 for r in results if r.status == EvalStatus.PASS)
        failed = sum(1 for r in results if r.status == EvalStatus.FAIL)
        errors = sum(1 for r in results if r.status == EvalStatus.ERROR)
        skipped = sum(1 for r in results if r.status == EvalStatus.SKIP)
        avg_lat = sum(r.latency_ms for r in results) / len(results) if results else 0.0
        return EvalReport(
            run_id=run_id,
            agent=cases[0].agent if cases else "unknown",
            total=len(cases),
            passed=passed,
            failed=failed,
            errors=errors,
            skipped=skipped,
            pass_rate=passed / len(cases) if cases else 0.0,
            avg_latency_ms=avg_lat,
            results=list(results),
            timestamp=datetime.now(timezone.utc).isoformat(),
        )

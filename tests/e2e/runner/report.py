#!/usr/bin/env python3
"""Агрегатор результатов pytest+playwright в .omc/e2e-results.jsonl.

Append-only JSONL: одна строка на тест.
{
  "ts": "2026-05-10T12:34:56Z",
  "suite": "api" | "ui",
  "scenario": "test_applicant_flow.py::test_applicant_flow[sid01_LLC_approved]",
  "status": "passed" | "failed" | "skipped",
  "duration_ms": 1234,
  "error": null | "traceback or message"
}
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def _now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_pytest_json(path: Path) -> list[dict[str, Any]]:
    if not path.exists() or path.stat().st_size == 0:
        return []
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return []
    rows: list[dict[str, Any]] = []
    for t in data.get("tests", []):
        outcome = t.get("outcome", "unknown")
        status = {"passed": "passed", "failed": "failed", "skipped": "skipped", "error": "failed"}.get(outcome, outcome)
        duration = t.get("duration", 0.0)
        error = None
        if status == "failed":
            call = t.get("call") or {}
            error = call.get("longrepr") or call.get("crash", {}).get("message")
        rows.append(
            {
                "suite": "api",
                "scenario": t.get("nodeid", "<unknown>"),
                "status": status,
                "duration_ms": int(duration * 1000),
                "error": error,
            }
        )
    return rows


def parse_playwright_json(path: Path) -> list[dict[str, Any]]:
    if not path.exists() or path.stat().st_size == 0:
        return []
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return []
    rows: list[dict[str, Any]] = []
    # Playwright JSON: suites[].{file, specs[].{title, tests[].{results[]}}}
    # рекурсивно пробегаем suites (может быть вложенность)
    def walk(suites: list[dict[str, Any]], parent_title: str = "") -> None:
        for s in suites:
            title = s.get("title", "")
            full = f"{parent_title}/{title}".strip("/")
            for spec in s.get("specs", []):
                spec_title = spec.get("title", "<unknown>")
                for t in spec.get("tests", []):
                    for r in t.get("results", []):
                        status = r.get("status", "unknown")
                        if status == "passed":
                            err = None
                        elif status == "skipped":
                            err = None
                        else:
                            status = "failed"
                            err_obj = r.get("error") or {}
                            err = err_obj.get("message") or err_obj.get("stack")
                        rows.append(
                            {
                                "suite": "ui",
                                "scenario": f"{full}::{spec_title}",
                                "status": status,
                                "duration_ms": int(r.get("duration", 0)),
                                "error": err,
                            }
                        )
            # вложенные сьюты
            walk(s.get("suites", []) or [], parent_title=full)

    walk(data.get("suites", []))
    return rows


def append_jsonl(rows: list[dict[str, Any]], output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    ts = _now_iso()
    with output.open("a", encoding="utf-8") as f:
        for r in rows:
            line = {"ts": ts, **r}
            f.write(json.dumps(line, ensure_ascii=False) + "\n")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--api-json", required=True)
    ap.add_argument("--ui-json", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    rows: list[dict[str, Any]] = []
    rows.extend(parse_pytest_json(Path(args.api_json)))
    rows.extend(parse_playwright_json(Path(args.ui_json)))

    if not rows:
        print("WARN: no test results parsed (api or ui report empty/malformed)", file=sys.stderr)

    append_jsonl(rows, Path(args.output))

    # Сводка для оператора
    by_status: dict[str, int] = {}
    for r in rows:
        by_status[r["status"]] = by_status.get(r["status"], 0) + 1
    total = len(rows)
    summary = " ".join(f"{k}={v}" for k, v in sorted(by_status.items()))
    print(f"aggregated: total={total} {summary}")
    print(f"appended to: {args.output}")
    # Exit code 1 если хоть один тест failed — для systemd OnFailure=.
    return 1 if by_status.get("failed", 0) > 0 else 0


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""Seed 10 разных компаний (заявителей онбординга) в tenant=demo через onboarding API.

Цепочка для каждой компании:
  POST /v1/applicants                                  → identity-service:8082
  POST /v1/applications                                → onboarding-orchestrator:8085
  POST /v1/documents (multipart, по 1-3 на заявку)     → document-service:8083
  POST /v1/applications/{id}/signals/documents-uploaded
  POST /v1/applications/{id}/signals/human-decision    (для approved/declined)

Скрипт идемпотентен на уровне applicants (handler возвращает существующего по
tenant_id+inn). Результаты прогона сохраняются в .omc/seed-results.json — повторный
запуск пропускает сценарии, у которых уже зафиксирован application_id.

Зависимости — только stdlib (urllib, json). Запускать с локальной машины:
  python3 scripts/seed-staging-companies.py
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

DEFAULT_BASE_URL = "http://206.204.106.28"
DEFAULT_TENANT = "demo"
DEFAULT_ACTOR = "seed-script"
RESULTS_PATH = Path(".omc/seed-results.json")

PORTS = {
    "identity": 8082,
    "document": 8083,
    "orchestrator": 8085,
    "risk": 8086,
    "ubo": 8097,
}

# Минимальный валидный PDF (~190 байт) для multipart-загрузки.
MIN_PDF: bytes = (
    b"%PDF-1.4\n"
    b"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
    b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
    b"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n"
    b"xref\n"
    b"0 4\n"
    b"0000000000 65535 f \n"
    b"0000000009 00000 n \n"
    b"0000000053 00000 n \n"
    b"0000000099 00000 n \n"
    b"trailer<</Size 4/Root 1 0 R>>\n"
    b"startxref\n"
    b"156\n"
    b"%%EOF\n"
)


@dataclass
class Scenario:
    sid: int
    name: str
    le_type: str  # IP | LLC | JSC | NPF
    okved: str
    le_inn: str  # ИНН юрлица (10 цифр) или физлица для ИП (12 цифр)
    le_ogrn: str
    applicant_inn: str  # 12 цифр (физлицо-заявитель)
    applicant_phone: str
    applicant_full_name: str
    documents: list[str]  # типы документов: passport|charter|protocol|extract|agreement
    risk_thresholds: dict[str, float]
    final_decision: dict[str, str] | None  # {decision, reviewer, reason} или None
    skip_documents: bool = False  # для draft-сценария: applicant+application и всё
    invalid_applicant_inn: bool = False  # для граничного кейса валидации
    note: str = ""


# 10 сценариев. ИНН/ОГРН — синтетические и не пересекаются с реальными организациями.
SCENARIOS: list[Scenario] = [
    Scenario(
        sid=1,
        name="ООО «Ромашка Трейд»",
        le_type="LLC",
        okved="47.11",
        le_inn="7701000001",
        le_ogrn="1027700000001",
        applicant_inn="770100000011",
        applicant_phone="+79991000001",
        applicant_full_name="Иванов Иван Иванович",
        documents=["passport", "charter", "protocol", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "Low risk, complete docs"},
        note="LLC happy path → approved",
    ),
    Scenario(
        sid=2,
        name="ООО «ТехноСофт»",
        le_type="LLC",
        okved="62.01",
        le_inn="7702000002",
        le_ogrn="1027700000002",
        applicant_inn="770200000022",
        applicant_phone="+79991000002",
        applicant_full_name="Соколова Анна Петровна",
        documents=["passport", "charter", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "IT, low risk"},
        note="LLC IT-компания → approved",
    ),
    Scenario(
        sid=3,
        name="ИП Петров А.С.",
        le_type="IP",
        okved="96.02",
        le_inn="770300000033",
        le_ogrn="304770000000033",
        applicant_inn="770300000033",
        applicant_phone="+79991000003",
        applicant_full_name="Петров Алексей Сергеевич",
        documents=["passport", "extract"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved", "reviewer": "seed-bot", "reason": "Sole prop, services"},
        note="IP services → approved",
    ),
    Scenario(
        sid=4,
        name="ИП Сидорова Е.В.",
        le_type="IP",
        okved="47.91",
        le_inn="770400000044",
        le_ogrn="304770000000044",
        applicant_inn="770400000044",
        applicant_phone="+79991000004",
        applicant_full_name="Сидорова Елена Викторовна",
        documents=["passport"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,  # без human-decision → остаётся в running/manual_review
        note="IP partial docs → manual_review",
    ),
    Scenario(
        sid=5,
        name="АО «МедСтрой»",
        le_type="JSC",
        okved="41.20",
        le_inn="7705000005",
        le_ogrn="1027700000005",
        applicant_inn="770500000055",
        applicant_phone="+79991000005",
        applicant_full_name="Морозов Дмитрий Олегович",
        documents=["passport", "charter", "protocol"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "approved_with_edd", "reviewer": "seed-bot", "reason": "Construction, mid-size, EDD required"},
        note="JSC construction → approved_with_edd",
    ),
    Scenario(
        sid=6,
        name="АО «Энергоинвест»",
        le_type="JSC",
        okved="35.11",
        le_inn="7706000006",
        le_ogrn="1027700000006",
        applicant_inn="770600000066",
        applicant_phone="+79991000006",
        applicant_full_name="Волкова Татьяна Михайловна",
        documents=["passport", "charter", "protocol", "extract", "agreement"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,  # большой АО → manual_review
        note="JSC large public → manual_review",
    ),
    Scenario(
        sid=7,
        name="ООО «Глобал Трейд Корп»",
        le_type="LLC",
        okved="46.90",
        le_inn="7707000007",
        le_ogrn="1027700000007",
        applicant_inn="770700000077",
        applicant_phone="+79991000007",
        applicant_full_name="Кузнецов Роман Андреевич",
        documents=["passport", "charter"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "declined", "reviewer": "seed-bot", "reason": "115-FZ sanction match"},
        note="LLC high risk → declined (115-ФЗ match)",
    ),
    Scenario(
        sid=8,
        name="ООО «Кипр Холдинг»",
        le_type="LLC",
        okved="64.99",
        le_inn="7708000008",
        le_ogrn="1027700000008",
        applicant_inn="770800000088",
        applicant_phone="+79991000008",
        applicant_full_name="Семёнов Павел Викторович",
        documents=["passport", "charter"],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision={"decision": "declined", "reviewer": "seed-bot", "reason": "Offshore UBO chain incomplete"},
        note="LLC offshore UBO → declined",
    ),
    Scenario(
        sid=9,
        name="ООО «Старт Айдиа»",
        le_type="LLC",
        okved="70.22",
        le_inn="7709000009",
        le_ogrn="1027700000009",
        applicant_inn="770900000099",
        applicant_phone="+79991000009",
        applicant_full_name="Орлова Мария Сергеевна",
        documents=[],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,
        skip_documents=True,
        note="LLC draft (no docs) → state=draft",
    ),
    Scenario(
        sid=10,
        name="ООО «Ошибка ИНН»",
        le_type="LLC",
        okved="47.11",
        le_inn="7710000010",
        le_ogrn="1027700000010",
        applicant_inn="ABC0000010",  # 10 chars, не цифры — упадёт на validate()
        applicant_phone="+79991000010",
        applicant_full_name="Тестов Эдж Кейсович",
        documents=[],
        risk_thresholds={"auto_approve_below": 0.30, "decline_above": 0.85},
        final_decision=None,
        skip_documents=True,
        invalid_applicant_inn=True,
        note="Boundary: invalid INN → 400 on POST /v1/applicants",
    ),
]


# ---------- HTTP helpers (stdlib only) ----------


class HttpError(Exception):
    def __init__(self, status: int, body: str, url: str):
        super().__init__(f"HTTP {status} {url}: {body[:200]}")
        self.status = status
        self.body = body
        self.url = url


def http_request(
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    json_body: Any = None,
    raw_body: bytes | None = None,
    content_type: str | None = None,
    timeout: float = 15.0,
) -> dict[str, Any]:
    body: bytes | None = None
    h = dict(headers or {})
    if json_body is not None:
        body = json.dumps(json_body, ensure_ascii=False).encode("utf-8")
        h.setdefault("Content-Type", "application/json")
    elif raw_body is not None:
        body = raw_body
        if content_type:
            h.setdefault("Content-Type", content_type)
    req = urllib.request.Request(url, data=body, method=method, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = resp.read()
            text = data.decode("utf-8", errors="replace") if data else ""
            return _parse_response(resp.status, text, url)
    except urllib.error.HTTPError as e:
        text = ""
        try:
            text = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        raise HttpError(e.code, text, url)


def _parse_response(status: int, text: str, url: str) -> dict[str, Any]:
    if not text:
        return {"_status": status}
    try:
        result = json.loads(text)
        if isinstance(result, dict):
            result.setdefault("_status", status)
            return result
        return {"_status": status, "_raw": result}
    except json.JSONDecodeError:
        return {"_status": status, "_raw": text}


def build_multipart(fields: dict[str, str], file_field: str, filename: str, file_bytes: bytes, mime: str) -> tuple[bytes, str]:
    boundary = "----aibankSeed" + uuid.uuid4().hex
    out = bytearray()
    for k, v in fields.items():
        out += f"--{boundary}\r\n".encode()
        out += f'Content-Disposition: form-data; name="{k}"\r\n\r\n'.encode()
        out += v.encode("utf-8") + b"\r\n"
    out += f"--{boundary}\r\n".encode()
    out += f'Content-Disposition: form-data; name="{file_field}"; filename="{filename}"\r\n'.encode()
    out += f"Content-Type: {mime}\r\n\r\n".encode()
    out += file_bytes + b"\r\n"
    out += f"--{boundary}--\r\n".encode()
    return bytes(out), f"multipart/form-data; boundary={boundary}"


# ---------- API actions ----------


def url(base: str, service: str, path: str) -> str:
    return f"{base}:{PORTS[service]}{path}"


def create_applicant(base: str, tenant: str, sc: Scenario, *, dry_run: bool) -> dict[str, Any]:
    payload = {
        "tenant_id": tenant,
        "inn": sc.applicant_inn,
        "phone": sc.applicant_phone,
        "full_name": sc.applicant_full_name,
        "consents": [{"type": "data_processing", "granted": True, "version": "1.0"}],
    }
    if dry_run:
        return {"_dry_run": True, "payload": payload}
    return http_request("POST", url(base, "identity", "/v1/applicants"), json_body=payload)


def create_application(base: str, tenant: str, sc: Scenario, applicant_id: str, *, dry_run: bool) -> dict[str, Any]:
    payload = {
        "tenant_id": tenant,
        "applicant_id": applicant_id,
        "legal_entity_type": sc.le_type,
        "channel": "web",
        "product_codes": ["current_account_rub"],
        "risk_thresholds": sc.risk_thresholds,
    }
    if dry_run:
        return {"_dry_run": True, "payload": payload}
    return http_request(
        "POST",
        url(base, "orchestrator", "/v1/applications"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body=payload,
    )


def upload_document(base: str, tenant: str, application_id: str, doc_type: str, *, dry_run: bool) -> dict[str, Any]:
    fields = {"tenant_id": tenant, "application_id": application_id, "type": doc_type}
    filename = f"{doc_type}_{application_id[:8]}.pdf"
    if dry_run:
        return {"_dry_run": True, "fields": fields, "filename": filename, "size": len(MIN_PDF)}
    body, ctype = build_multipart(fields, "file", filename, MIN_PDF, "application/pdf")
    return http_request(
        "POST",
        url(base, "document", "/v1/documents"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        raw_body=body,
        content_type=ctype,
    )


def signal_documents_uploaded(base: str, tenant: str, application_id: str, doc_refs: list[dict], *, dry_run: bool) -> dict[str, Any]:
    payload = {"tenant_id": tenant, "documents": doc_refs}
    if dry_run:
        return {"_dry_run": True, "payload": payload}
    return http_request(
        "POST",
        url(base, "orchestrator", f"/v1/applications/{application_id}/signals/documents-uploaded"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body=payload,
    )


def signal_human_decision(base: str, tenant: str, application_id: str, decision: dict[str, str], *, dry_run: bool) -> dict[str, Any]:
    payload = {"tenant_id": tenant, **decision}
    if dry_run:
        return {"_dry_run": True, "payload": payload}
    return http_request(
        "POST",
        url(base, "orchestrator", f"/v1/applications/{application_id}/signals/human-decision"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body=payload,
    )


# ---------- Scenario runner ----------


@dataclass
class ScenarioResult:
    sid: int
    name: str
    note: str
    status: str  # ok | partial | failed | skipped | expected_failure
    applicant_id: str = ""
    application_id: str = ""
    document_ids: list[str] = field(default_factory=list)
    decision_sent: bool = False
    error: str = ""


def run_scenario(base: str, tenant: str, sc: Scenario, *, dry_run: bool) -> ScenarioResult:
    res = ScenarioResult(sid=sc.sid, name=sc.name, note=sc.note, status="failed")
    # 1. Applicant
    try:
        a = create_applicant(base, tenant, sc, dry_run=dry_run)
    except HttpError as e:
        if sc.invalid_applicant_inn and e.status == 400:
            res.status = "expected_failure"
            res.error = e.body.strip()
            return res
        res.error = f"applicant: {e}"
        return res
    if dry_run:
        res.applicant_id = "(dry-run)"
    else:
        res.applicant_id = a.get("id", "")
        if not res.applicant_id:
            res.error = f"applicant: no id in response: {a}"
            return res
    # 2. Application
    try:
        app = create_application(base, tenant, sc, res.applicant_id, dry_run=dry_run)
    except HttpError as e:
        res.error = f"application: {e}"
        return res
    if dry_run:
        res.application_id = "(dry-run)"
    else:
        res.application_id = app.get("application_id", "")
        if not res.application_id:
            res.error = f"application: no application_id: {app}"
            return res
    # 3. Documents
    doc_refs: list[dict[str, Any]] = []
    if not sc.skip_documents:
        for dt in sc.documents:
            try:
                doc = upload_document(base, tenant, res.application_id, dt, dry_run=dry_run)
            except HttpError as e:
                res.error = f"document {dt}: {e}"
                res.status = "partial"
                return res
            if dry_run:
                continue
            doc_id = doc.get("id", "")
            if doc_id:
                res.document_ids.append(doc_id)
                doc_refs.append({"id": doc_id, "type": dt, "storage_path": doc.get("storage_path", "")})
        # 4. Signal documents-uploaded (только если действительно загружали)
        if doc_refs:
            try:
                signal_documents_uploaded(base, tenant, res.application_id, doc_refs, dry_run=dry_run)
            except HttpError as e:
                res.error = f"signal documents-uploaded: {e}"
                res.status = "partial"
                return res
    # 5. Human decision (опционально)
    if sc.final_decision:
        # Дадим воркфлоу время дойти до состояния, принимающего сигнал.
        if not dry_run:
            time.sleep(0.5)
        try:
            signal_human_decision(base, tenant, res.application_id, sc.final_decision, dry_run=dry_run)
            res.decision_sent = True
        except HttpError as e:
            # Workflow может ещё не быть в manual_review — это ожидаемо для draft/просто созданных.
            res.error = f"signal human-decision: {e}"
            res.status = "partial"
            return res
    res.status = "ok"
    return res


# ---------- Persistence ----------


def load_results() -> dict[str, dict[str, Any]]:
    if not RESULTS_PATH.exists():
        return {}
    try:
        return json.loads(RESULTS_PATH.read_text())
    except Exception:
        return {}


def save_results(data: dict[str, dict[str, Any]]) -> None:
    RESULTS_PATH.parent.mkdir(parents=True, exist_ok=True)
    RESULTS_PATH.write_text(json.dumps(data, ensure_ascii=False, indent=2))


# ---------- CLI ----------


def parse_scenario_filter(value: str | None) -> set[int] | None:
    if not value:
        return None
    out: set[int] = set()
    for part in value.split(","):
        part = part.strip()
        if not part:
            continue
        out.add(int(part))
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base-url", default=DEFAULT_BASE_URL, help=f"Базовый URL без порта (default: {DEFAULT_BASE_URL})")
    ap.add_argument("--tenant", default=DEFAULT_TENANT)
    ap.add_argument("--scenarios", help="Список ID сценариев через запятую, например '1,3,7'")
    ap.add_argument("--dry-run", action="store_true", help="Печатать payload'ы без HTTP-вызовов")
    ap.add_argument("--force", action="store_true", help="Игнорировать кэш .omc/seed-results.json")
    args = ap.parse_args()

    sids = parse_scenario_filter(args.scenarios)
    cache = {} if args.force else load_results()

    selected = [s for s in SCENARIOS if sids is None or s.sid in sids]
    print(f"[seed] base={args.base_url} tenant={args.tenant} scenarios={len(selected)} dry_run={args.dry_run}")
    print()

    results: list[ScenarioResult] = []
    for sc in selected:
        cached = cache.get(str(sc.sid))
        if (
            not args.dry_run
            and not args.force
            and cached
            and cached.get("status") in {"ok", "expected_failure"}
            and cached.get("application_id")
        ):
            print(f"[skip] #{sc.sid:>2} {sc.name} — already seeded as {cached['application_id']}")
            continue

        print(f"[run]  #{sc.sid:>2} {sc.name} ({sc.note})")
        res = run_scenario(args.base_url, args.tenant, sc, dry_run=args.dry_run)
        results.append(res)
        if res.status == "ok":
            print(f"       ✓ applicant={res.applicant_id[:8]} application={res.application_id} docs={len(res.document_ids)} decision={res.decision_sent}")
        elif res.status == "expected_failure":
            print(f"       ✓ expected validation failure: {res.error[:100]}")
        elif res.status == "partial":
            print(f"       ~ partial: applicant={res.applicant_id[:8]} application={res.application_id} err={res.error[:120]}")
        else:
            print(f"       ✗ failed: {res.error[:200]}")
        # Persist по мере выполнения, чтобы прерывание не теряло прогресс.
        if not args.dry_run:
            cache[str(sc.sid)] = res.__dict__.copy()
            save_results(cache)

    # Сводка
    print()
    print("=" * 78)
    print(f"{'#':>2}  {'name':<32} {'status':<18} application_id")
    print("-" * 78)
    summary = {"ok": 0, "expected_failure": 0, "partial": 0, "failed": 0, "skipped": 0}
    for sc in selected:
        cached = cache.get(str(sc.sid))
        if not cached:
            print(f"{sc.sid:>2}  {sc.name[:32]:<32} {'(skipped)':<18} —")
            summary["skipped"] += 1
            continue
        st = cached.get("status", "?")
        summary[st] = summary.get(st, 0) + 1
        print(f"{sc.sid:>2}  {sc.name[:32]:<32} {st:<18} {cached.get('application_id', '—')}")
    print("-" * 78)
    print(f"  ok={summary['ok']} expected_failure={summary['expected_failure']} partial={summary['partial']} failed={summary['failed']} skipped={summary['skipped']}")
    print(f"  results saved to {RESULTS_PATH}")
    return 0 if summary["failed"] == 0 else 1


if __name__ == "__main__":
    sys.exit(main())

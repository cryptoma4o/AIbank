"""HTTP helpers для E2E API-тестов AIbank.

Используется urllib (stdlib only) — без зависимостей. Если в будущем потребуется
async или HTTP/2 — мигрировать на httpx.
"""

from __future__ import annotations

import json
import time
import urllib.error
import urllib.request
import uuid
from typing import Any

# Внутренние порты сервисов (через docker-compose). При прогоне на staging-сервере
# через --network host эти порты доступны на localhost:.
PORTS: dict[str, int] = {
    "identity": 8082,
    "document": 8083,
    "orchestrator": 8085,
    "risk": 8086,
    "ubo": 8097,
    "audit": 8081,
    "api_gateway": 8000,
    "bff_admin": 8092,
    "bff_onboarding": 8091,
    "llm_gateway": 8100,
    "rag_service": 8105,
}

DEFAULT_ACTOR = "e2e-test"


class HttpError(Exception):
    def __init__(self, status: int, body: str, url: str):
        super().__init__(f"HTTP {status} {url}: {body[:200]}")
        self.status = status
        self.body = body
        self.url = url


def url(base: str, service: str, path: str) -> str:
    """Собрать URL: base ('http://localhost') + порт сервиса + path."""
    return f"{base}:{PORTS[service]}{path}"


def http_request(
    method: str,
    target_url: str,
    *,
    headers: dict[str, str] | None = None,
    json_body: Any = None,
    raw_body: bytes | None = None,
    content_type: str | None = None,
    timeout: float = 15.0,
) -> dict[str, Any]:
    """Минималистичный HTTP-клиент с raise-on-4xx/5xx через HttpError."""
    body: bytes | None = None
    h = dict(headers or {})
    if json_body is not None:
        body = json.dumps(json_body, ensure_ascii=False).encode("utf-8")
        h.setdefault("Content-Type", "application/json")
    elif raw_body is not None:
        body = raw_body
        if content_type:
            h.setdefault("Content-Type", content_type)
    req = urllib.request.Request(target_url, data=body, method=method, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = resp.read()
            text = data.decode("utf-8", errors="replace") if data else ""
            return _parse_response(resp.status, text)
    except urllib.error.HTTPError as e:
        text = ""
        try:
            text = e.read().decode("utf-8", errors="replace")
        except Exception:
            pass
        raise HttpError(e.code, text, target_url) from e


def _parse_response(status: int, text: str) -> dict[str, Any]:
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


def build_multipart(
    fields: dict[str, str],
    file_field: str,
    filename: str,
    file_bytes: bytes,
    mime: str,
) -> tuple[bytes, str]:
    """Собрать multipart/form-data с одним файлом."""
    boundary = "----aibankE2E" + uuid.uuid4().hex
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


# ---------- Высокоуровневые actions для тестов ----------


def create_applicant(base: str, tenant: str, *, inn: str, phone: str, full_name: str) -> dict[str, Any]:
    payload = {
        "tenant_id": tenant,
        "inn": inn,
        "phone": phone,
        "full_name": full_name,
        "consents": [{"type": "data_processing", "granted": True, "version": "1.0"}],
    }
    return http_request("POST", url(base, "identity", "/v1/applicants"), json_body=payload)


def create_application(
    base: str,
    tenant: str,
    *,
    applicant_id: str,
    le_type: str,
    risk_thresholds: dict[str, float],
    products: list[str] | None = None,
) -> dict[str, Any]:
    payload = {
        "tenant_id": tenant,
        "applicant_id": applicant_id,
        "legal_entity_type": le_type,
        "channel": "web",
        "product_codes": products or ["current_account_rub"],
        "risk_thresholds": risk_thresholds,
    }
    return http_request(
        "POST",
        url(base, "orchestrator", "/v1/applications"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body=payload,
    )


def create_or_get_application(
    base: str,
    tenant: str,
    *,
    applicant_id: str,
    le_type: str,
    risk_thresholds: dict[str, float],
    products: list[str] | None = None,
) -> tuple[str, bool]:
    """Идемпотентный wrapper над create_application.

    Возвращает (application_id, was_created).
    Если orchestrator вернул 409 'applicant_has_application' — извлекает
    existing_application_id и возвращает was_created=False.
    """
    try:
        resp = create_application(
            base,
            tenant,
            applicant_id=applicant_id,
            le_type=le_type,
            risk_thresholds=risk_thresholds,
            products=products,
        )
        return resp.get("application_id", ""), True
    except HttpError as e:
        if e.status == 409:
            try:
                body = json.loads(e.body)
                existing = body.get("existing_application_id") or ""
                if existing:
                    return existing, False
            except json.JSONDecodeError:
                pass
        raise


def upload_document(base: str, tenant: str, application_id: str, doc_type: str, payload: bytes) -> dict[str, Any]:
    fields = {"tenant_id": tenant, "application_id": application_id, "type": doc_type}
    filename = f"{doc_type}_{application_id[:8]}.pdf"
    body, ctype = build_multipart(fields, "file", filename, payload, "application/pdf")
    return http_request(
        "POST",
        url(base, "document", "/v1/documents"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        raw_body=body,
        content_type=ctype,
    )


def signal_documents_uploaded(base: str, tenant: str, application_id: str, doc_refs: list[dict]) -> dict[str, Any]:
    return http_request(
        "POST",
        url(base, "orchestrator", f"/v1/applications/{application_id}/signals/documents-uploaded"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body={"tenant_id": tenant, "documents": doc_refs},
    )


def signal_human_decision(base: str, tenant: str, application_id: str, decision: dict[str, str]) -> dict[str, Any]:
    return http_request(
        "POST",
        url(base, "orchestrator", f"/v1/applications/{application_id}/signals/human-decision"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
        json_body={"tenant_id": tenant, **decision},
    )


def get_application(base: str, tenant: str, application_id: str) -> dict[str, Any]:
    return http_request(
        "GET",
        url(base, "orchestrator", f"/v1/applications/{application_id}"),
        headers={"X-Tenant-ID": tenant, "X-Actor-ID": DEFAULT_ACTOR},
    )


def poll_application_status(
    base: str,
    tenant: str,
    application_id: str,
    *,
    expected_terminal: set[str],
    timeout_s: float = 30.0,
    interval_s: float = 1.0,
) -> dict[str, Any]:
    """Опрашивает /v1/applications/{id} пока state не попадёт в expected_terminal или не истечёт timeout."""
    deadline = time.monotonic() + timeout_s
    last: dict[str, Any] = {}
    while time.monotonic() < deadline:
        try:
            last = get_application(base, tenant, application_id)
        except HttpError:
            time.sleep(interval_s)
            continue
        state = (last.get("state") or last.get("status") or "").lower()
        if state in expected_terminal:
            return last
        time.sleep(interval_s)
    raise TimeoutError(
        f"poll_application_status: did not reach {expected_terminal} in {timeout_s}s. "
        f"Last state={last.get('state') or last.get('status')!r}"
    )


# ---------- Audit / admin ----------


def list_audit_events(base: str, tenant: str, *, entity_id: str | None = None, limit: int = 50) -> dict[str, Any]:
    qs = f"?tenant_id={tenant}&limit={limit}"
    if entity_id:
        qs += f"&entity_id={entity_id}"
    return http_request("GET", url(base, "audit", f"/v1/events{qs}"))

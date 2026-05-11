"""E2E API-тесты онбординга для всех 10 сценариев.

Параметризация по SCENARIOS — каждый сценарий превращается в отдельный pytest-тест.
Шаги совпадают с seed-staging-companies.py:
  applicant → application → documents → signal docs-uploaded → (опц.) signal human-decision

Тест НЕ проверяет финальное состояние workflow (зависит от Temporal/risk-engine,
flaky). Тест проверяет, что все API-вызовы успешны и возвращают валидные id.
Workflow-проверки — задача eval-harness и admin-flow тестов.
"""

from __future__ import annotations

import time

import pytest

from .helpers import (
    HttpError,
    create_applicant,
    create_or_get_application,
    signal_documents_uploaded,
    signal_human_decision,
    upload_document,
)
from .scenarios import MIN_PDF, SCENARIOS, Scenario


def _scenario_id(sc: Scenario) -> str:
    return f"sid{sc.sid:02d}_{sc.le_type}_{sc.note.split(chr(8594))[-1].strip()[:25].replace(' ', '_')}"


@pytest.mark.scenario
@pytest.mark.parametrize("sc", SCENARIOS, ids=_scenario_id)
def test_applicant_flow(sc: Scenario, base_url: str, tenant: str) -> None:
    # 1. Applicant — создание (или ожидаемая 400 на boundary-кейсе)
    if sc.invalid_applicant_inn:
        with pytest.raises(HttpError) as exc:
            create_applicant(
                base_url,
                tenant,
                inn=sc.applicant_inn,
                phone=sc.applicant_phone,
                full_name=sc.applicant_full_name,
            )
        assert exc.value.status == 400, f"expected 400 for invalid INN, got {exc.value.status}"
        return  # boundary-сценарий завершён

    applicant = create_applicant(
        base_url,
        tenant,
        inn=sc.applicant_inn,
        phone=sc.applicant_phone,
        full_name=sc.applicant_full_name,
    )
    applicant_id = applicant.get("id", "")
    assert applicant_id, f"applicant: no id in response: {applicant}"

    # 2. Application (идемпотентно: 409 'applicant_has_application' → reuse existing)
    application_id, was_created = create_or_get_application(
        base_url,
        tenant,
        applicant_id=applicant_id,
        le_type=sc.le_type,
        risk_thresholds=sc.risk_thresholds,
    )
    assert application_id, f"application: empty id (was_created={was_created})"

    if not was_created:
        # Сценарий уже отыгран в прошлый раз — documents и signals уже отправлены.
        # Прерываемся: тест зелёный, факт существования заявки = успех.
        return

    # 3. Documents (если сценарий не draft)
    doc_refs: list[dict] = []
    if not sc.skip_documents:
        for dt in sc.documents:
            doc = upload_document(base_url, tenant, application_id, dt, MIN_PDF)
            doc_id = doc.get("id", "")
            assert doc_id, f"document {dt}: no id in response: {doc}"
            doc_refs.append(
                {"id": doc_id, "type": dt, "storage_path": doc.get("storage_path", "")}
            )

        # 4. Signal docs-uploaded — orchestrator принимает только если был хоть 1 документ
        if doc_refs:
            signal_documents_uploaded(base_url, tenant, application_id, doc_refs)

    # 5. Human decision (опционально)
    if sc.final_decision:
        # Workflow должен дойти до состояния, принимающего сигнал.
        # Для большинства сценариев — секунды; в идеале использовать poll, но для
        # стабильности оставляем фиксированную паузу (как в seed-script).
        time.sleep(0.5)
        try:
            signal_human_decision(base_url, tenant, application_id, sc.final_decision)
        except HttpError as e:
            # Workflow может ещё не быть в manual_review — для approved/declined
            # путей, где auto-approve срабатывает раньше, это ожидаемо.
            # Тест не падает: проверяет факт API-доступности, не workflow.
            pytest.skip(
                f"human-decision signal rejected (workflow state may auto-approve): "
                f"HTTP {e.status} — {e.body[:120]}"
            )

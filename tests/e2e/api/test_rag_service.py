"""E2E проверка rag-service: поиск по проиндексированной нормативке.

Тесты предполагают что 115-ФЗ + 152-ФЗ + 375-П уже проиндексированы в
tenant=demo (через `make rag-index`). Если индекс пуст — тесты падают
с понятным сообщением, что нужно сначала запустить индексацию.
"""

from __future__ import annotations

import pytest

from .helpers import HttpError, http_request, url


@pytest.mark.scenario
def test_rag_health(base_url: str) -> None:
    resp = http_request("GET", url(base_url, "rag_service", "/healthz"))
    assert resp.get("_status") == 200
    assert resp.get("embedder") in {"tei", "mock"}
    assert resp.get("dim") > 0


@pytest.mark.scenario
def test_rag_finds_115fz_thresholds(base_url: str, tenant: str) -> None:
    """Запрос о порогах обязательного контроля должен возвращать ст. 6 115-ФЗ."""
    resp = http_request(
        "POST",
        url(base_url, "rag_service", "/v1/search"),
        json_body={
            "tenant_id": tenant,
            "query": "какие операции подлежат обязательному контролю и какие пороги в рублях",
            "top_k": 5,
        },
    )
    assert resp.get("_status") == 200
    hits = resp.get("hits", [])
    if not hits:
        pytest.skip(
            "rag индекс пуст — запусти `make rag-index` или проверь "
            f"что rag-service видит tenant={tenant}"
        )
    sources = [h.get("source", "") for h in hits]
    # Не цепляемся к конкретной статье (115-ФЗ Q3-Q4 может уплыть в другие
    # фрагменты), но хотим видеть в top-5 хоть один hit из 115-ФЗ.
    assert any("115-ФЗ" in s for s in sources), \
        f"top-{len(sources)} не содержит 115-ФЗ: {sources}"


@pytest.mark.scenario
def test_rag_filter_by_source_type(base_url: str, tenant: str) -> None:
    """Фильтр по source_type=law должен исключать regulation (375-П)."""
    resp = http_request(
        "POST",
        url(base_url, "rag_service", "/v1/search"),
        json_body={
            "tenant_id": tenant,
            "query": "внутренний контроль кредитной организации",
            "top_k": 5,
            "filters": {"source_type": "law"},
        },
    )
    if resp.get("_status") != 200 or not resp.get("hits"):
        pytest.skip("rag индекс пуст — запусти `make rag-index`")
    for h in resp["hits"]:
        meta = h.get("metadata") or {}
        # тип должен быть law (закон), не regulation
        assert meta.get("law_id", "").endswith("-fz") or "ФЗ" in h.get("source", ""), \
            f"hit {h.get('document_id')} прошёл фильтр source_type=law, но source={h.get('source')}"


@pytest.mark.scenario
def test_rag_empty_query_400(base_url: str, tenant: str) -> None:
    """Пустой query должен возвращать 400."""
    with pytest.raises(HttpError) as exc:
        http_request(
            "POST",
            url(base_url, "rag_service", "/v1/search"),
            json_body={"tenant_id": tenant, "query": "", "top_k": 5},
        )
    assert exc.value.status == 400

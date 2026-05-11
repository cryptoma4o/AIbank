"""E2E проверка llm-gateway: реальный LLM (не mock) отвечает через role-routing.

После переключения staging с LLM_GATEWAY_FORCE_MOCK=1 на staging-ollama.yaml
gateway должен вызывать Ollama и возвращать осмысленный русский ответ.
Mock-backend возвращает строку '[mock:<name>] echoing prompt digest=...'
(см. ai/llm-gateway/core/backends.py:MockBackend) — этот тест валит проверку
при mock-ответе.

Если на сервере Ollama не настроен и стоит fallback на mock — этот тест
ожидаемо упадёт. Это намеренно: тест нужен как монитор "real LLM is live".
"""

from __future__ import annotations

import pytest

from .helpers import http_request, url


@pytest.mark.scenario
def test_ru_chat_role_returns_real_llm(base_url: str, tenant: str) -> None:
    """role:ru-chat должен дать реальный (не mock) ответ от T-Lite."""
    resp = http_request(
        "POST",
        url(base_url, "llm_gateway", "/v1/chat/completions"),
        headers={"X-Tenant-Id": tenant},
        json_body={
            "model": "role:ru-chat",
            "messages": [
                {"role": "user", "content": "Ответь одним словом: 2+2?"}
            ],
            "max_tokens": 15,  # минимально для CPU staging
            "temperature": 0.1,
        },
        timeout=300,  # cold-start на CPU x86-v1 без AVX2 ~70 сек
    )
    assert resp.get("_status") == 200, f"unexpected status: {resp}"
    choices = resp.get("choices") or []
    assert choices, f"no choices in response: {resp}"
    content = (choices[0].get("message") or {}).get("content", "")

    # MockBackend.chat_completions всегда префиксит ответ "[mock:...]"
    assert "[mock:" not in content, f"got mock response, expected real LLM: {content!r}"

    # Реальный ответ может быть короткий ("4", "Четыре") — главное не mock-маркер.
    assert content.strip(), f"пустой ответ: {content!r}"

    # Usage должен быть заполнен реальным числом токенов.
    usage = resp.get("usage") or {}
    assert int(usage.get("completion_tokens", 0)) > 0, f"no tokens reported: {usage}"

    # Gateway проставляет свой metadata: ADR-0010.
    gw = resp.get("aibank_gateway") or {}
    assert gw.get("backend") and gw["backend"] != "mock", f"backend was mock: {gw}"
    assert gw.get("role") == "ru-chat"


@pytest.mark.scenario
def test_llm_gateway_models_list_contains_real_models(base_url: str) -> None:
    """GET /v1/models должен показать tlite и qwen-vl (не только mock)."""
    resp = http_request("GET", url(base_url, "llm_gateway", "/v1/models"))
    assert resp.get("_status") == 200
    ids = {m.get("id") for m in resp.get("data", []) if isinstance(m, dict)}
    # Проверяем наличие хотя бы одной реальной модели из staging-ollama.yaml.
    assert ids & {"tlite", "qwen-vl"}, f"no Ollama models registered, got: {ids}"

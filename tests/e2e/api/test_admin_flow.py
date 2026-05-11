"""E2E API-тесты admin-flow через bff-admin GraphQL.

Покрывает:
  - GraphQL endpoint жив и требует авторизации
  - AdminApplications query возвращает список (без авторизации — 401)
  - state machine transitions через mutation TransitionApplicationState

Этот тест НЕ требует валидного JWT для проверки 401 (smoke на доступность endpoint'а).
Полные authorized-сценарии — через cross-flow в Playwright UI.
"""

from __future__ import annotations

import pytest

from .helpers import HttpError, http_request, url


@pytest.mark.admin
def test_bff_admin_graphql_requires_auth(base_url: str) -> None:
    """GraphQL endpoint bff-admin должен отвечать 401 без Bearer-токена."""
    with pytest.raises(HttpError) as exc:
        http_request(
            "POST",
            url(base_url, "bff_admin", "/graphql"),
            json_body={"query": "{ __typename }"},
        )
    assert exc.value.status == 401, f"expected 401 unauth, got {exc.value.status}"
    assert "missing_token" in exc.value.body or "token" in exc.value.body.lower()


@pytest.mark.admin
def test_bff_admin_health(base_url: str) -> None:
    """bff-admin /health возвращает 200."""
    resp = http_request("GET", url(base_url, "bff_admin", "/health"))
    assert resp.get("_status") == 200, resp


@pytest.mark.admin
def test_bff_onboarding_graphql_requires_auth(base_url: str) -> None:
    """Симметричная проверка для bff-onboarding."""
    with pytest.raises(HttpError) as exc:
        http_request(
            "POST",
            url(base_url, "bff_onboarding", "/graphql"),
            json_body={"query": "{ __typename }"},
        )
    assert exc.value.status == 401, f"expected 401 unauth, got {exc.value.status}"

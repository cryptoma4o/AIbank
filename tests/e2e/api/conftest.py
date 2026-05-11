"""Pytest fixtures для E2E API-тестов AIbank.

Конфигурация через env:
  E2E_BASE_URL  — базовый URL без порта (default http://localhost)
  E2E_TENANT    — id тенанта в БД (default demo)

Запуск:
  pytest tests/e2e/api -v
  E2E_BASE_URL=http://206.204.106.28 pytest tests/e2e/api -v -k applicant
"""

from __future__ import annotations

import os
import time
import urllib.error
import urllib.request

import pytest


@pytest.fixture(scope="session")
def base_url() -> str:
    return os.environ.get("E2E_BASE_URL", "http://localhost").rstrip("/")


@pytest.fixture(scope="session")
def tenant() -> str:
    return os.environ.get("E2E_TENANT", "demo")


@pytest.fixture(scope="session", autouse=True)
def wait_for_stack(base_url: str) -> None:
    """Перед прогоном дождаться, что core-сервисы здоровы.

    Без этого тесты падают с ConnectionError, если стек ещё стартует
    (например, после `make up` или после restart docker).
    """
    endpoints = [
        f"{base_url}:8080/health",  # tenant
        f"{base_url}:8081/health",  # audit
        f"{base_url}:8082/health",  # identity
        f"{base_url}:8085/health",  # orchestrator
        f"{base_url}:8083/health",  # document
    ]
    deadline = time.monotonic() + 60.0
    for ep in endpoints:
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(ep, timeout=2) as resp:
                    if 200 <= resp.status < 300:
                        break
            except (urllib.error.URLError, OSError):
                pass
            time.sleep(1)
        else:
            pytest.fail(f"E2E setup: {ep} not healthy within 60s. Run `make up` first.")

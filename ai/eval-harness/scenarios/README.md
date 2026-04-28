# Scenarios — end-to-end synthetic eval

Сценарии проверяют **связку** сервисов на полном пути заявителя
(`draft → account_opened`), а не отдельный агент. Все HTTP-вызовы
перехватываются `respx` — реальные backend'ы запускать не нужно.

## Состав v1

| Сценарий | Шагов | Конечное состояние | Назначение |
|---|---|---|---|
| `happy_path` | 9 | `account_opened` | Smoke-проверка: orchestrator → identity → document → reconciliation → UBO → risk → decision → ABS → audit |

## Шаги happy_path

1. Регистрация заявителя (`identity-service`)
2. Создание заявки (`onboarding-orchestrator`, → `identifying`)
3. Загрузка документов (`document-service`, → `collecting_documents`)
4. Сверка (`agent-reconciliation`, → `validating`)
5. Раскрытие УБО (`agent-ubo-tracing`)
6. Скоринг (`agent-risk-scoring`, → `risk_assessing`)
7. Авто-решение (`onboarding-orchestrator`, → `approved`)
8. Открытие счёта (`abs-connector` → `cft` adapter, → `account_opened`)
9. Проверка цепочки аудита (`audit-verifier`)

State machine — источник истины: `services/onboarding-orchestrator/internal/domain/state.go`,
схема в `docs/domain-model.md` § 2.2.

## Запуск

```bash
cd ai/eval-harness
pip install -e .[dev]
pip install respx
pytest tests/test_scenarios.py -v
```

CLI-обёртка:

```bash
python -m harness.cli scenario list
python -m harness.cli scenario run happy_path
```

## Бюджеты

- Сценарий целиком — **< 5 секунд** (mock-only, проверяется в `test_scenario_under_5_seconds`)
- Отдельный шаг — **< 1 секунда** (см. `test_scenario_under_5_seconds`)
- Реальные latency-budget'ы по шагам — TODO (см. ниже)

## Как добавить новый сценарий

1. Создать файл `scenarios/<name>.py` со списком `Step`'ов и финальной
   `Scenario`-сборкой (см. `happy_path.py` как референс).
2. Каждый шаг — `async def step(ctx) -> dict`, регистрирует свой
   `respx.mock(...)`, делает один `httpx`-вызов, возвращает payload и
   пушит запись в `ctx['audit_events']`.
3. Re-export константы из `scenarios/__init__.py`.
4. Добавить test-кейсы в `tests/test_scenarios.py`.

## Roadmap

- `high_risk_decline.py` — score > 75 → `declined`, ABS не вызывается
- `requires_more_info.py` — middle-flow human review (`requires_more_info`
  → `waiting_for_client` → возврат на `validating`)
- `manual_review.py` — пограничный риск, проверка handoff'а в комплаенс-портал
- `abs_compensation.py` — `opening_account` → ошибка АБС → откат в `approved`
- Per-step latency budgets (p50/p95) с warn-mode'ом в CI

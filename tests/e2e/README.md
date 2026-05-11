# AIbank E2E Tests

Автономная система E2E-тестирования AIbank, эмулирующая работу пользователей
(applicant + bank-operator) против live-стека на staging-сервере.

## Что покрывается

**API (15 тестов, pytest, ~2 секунды)** — `tests/e2e/api/`:
- 10 параметризованных сценариев онбординга: LLC/IP/JSC × approved/declined/manual_review/draft/boundary
- admin-flow smoke: bff-admin GraphQL требует auth, /health отвечает
- audit-trail: запись события → чтение через GET /v1/events, проверка иммутабельности

**UI (4 spec'a, Playwright, ~10 секунд)** — `tests/e2e/ui/`:
- `applicant-register-login.spec.ts` — публичные страницы web-onboarding (/login, /register)
- `applicant-new-application.spec.ts` — авторизованная зона (seedFakeAuth), /applications + /applications/new
- `admin-transitions.spec.ts` — web-admin /login + /applications
- `cross-flow.spec.ts` — оба фронта живы, оба BFF возвращают 401 (а не 502)

## Single source of truth

Все 10 сценариев онбординга описаны в [api/scenarios.py](api/scenarios.py).
Тот же файл импортируется и `scripts/seed-staging-companies.py` —
добавил новый сценарий = он сразу покрыт тестом и сидится через скрипт.

## Запуск

### Самый быстрый: API-only, без Docker (~2 секунды)

```bash
# На staging-сервере (localhost):
cd /opt/aibank && make e2e-staging-api

# С локальной машины против staging:
E2E_BASE_URL=http://206.204.106.28 make e2e-staging-api
```

### Полный: Docker (API + UI)

```bash
# Соберёт ~1.5GB image (playwright base + python), запустит всё.
make e2e-staging
```

### Только UI (с локальной машины)

```bash
cd tests/e2e/ui
npm install && npx playwright install chromium
E2E_BASE_URL=http://206.204.106.28 npx playwright test
```

## Автономный прогон (cron)

systemd timer запускает Docker-runner каждый час и пишет в
`/opt/aibank/.omc/e2e-results.jsonl`.

```bash
# Установка (один раз):
make e2e-staging-install-timer

# Просмотр статуса:
ssh aibank 'systemctl list-timers aibank-e2e.timer'
ssh aibank 'journalctl -u aibank-e2e -f'
ssh aibank 'tail -f /opt/aibank/.omc/e2e-results.jsonl'

# Ручной запуск:
ssh aibank 'systemctl start aibank-e2e.service'
```

## Формат лога результатов

`.omc/e2e-results.jsonl` — append-only JSONL, одна строка на тест:

```json
{
  "ts": "2026-05-10T08:00:00Z",
  "suite": "api",
  "scenario": "tests/e2e/api/test_applicant_flow.py::test_applicant_flow[sid01_LLC_approved]",
  "status": "passed",
  "duration_ms": 1234,
  "error": null
}
```

Анализ:
```bash
# Сколько падений за сутки:
grep '"status":"failed"' .omc/e2e-results.jsonl | wc -l

# Самые медленные тесты:
jq -r '"\(.duration_ms)\t\(.scenario)"' .omc/e2e-results.jsonl | sort -rn | head

# История одного сценария:
jq -c 'select(.scenario | contains("sid07"))' .omc/e2e-results.jsonl
```

## Принципы

- **Идемпотентность**: повторные прогоны не падают. `create_application`
  ловит 409 `applicant_has_application` и возвращает существующий
  application_id — тест в этом случае ранее завершается успехом.
- **Без моков**: тесты бьют по реальным сервисам через 127.0.0.1:PORT.
- **Read-only для prod-кода**: тесты живут в `tests/`, не лезут в `services/`.

## Архитектура

```
tests/e2e/
├── api/                  # pytest — http через urllib (stdlib)
│   ├── scenarios.py      # 10 кейсов: ИНН, ОГРН, документы, ожидаемые исходы
│   ├── helpers.py        # HTTP, multipart, create_or_get_application (idempotent)
│   ├── conftest.py       # base_url, tenant, wait_for_stack fixtures
│   ├── test_applicant_flow.py
│   ├── test_admin_flow.py
│   └── test_audit_trail.py
├── ui/                   # Playwright — без MSW, против live стека
│   ├── playwright.config.ts   # 2 проекта: onboarding (3000), admin (3001)
│   ├── fixtures/auth.ts       # seedFakeAuth (smoke) + loginViaApi (real)
│   └── specs/
│       ├── applicant-register-login.spec.ts
│       ├── applicant-new-application.spec.ts
│       ├── admin-transitions.spec.ts
│       └── cross-flow.spec.ts
├── runner/
│   ├── run.sh            # pytest + playwright → /tmp/*.json → report.py
│   └── report.py         # парсит JSON, эмитит в .omc/e2e-results.jsonl
└── docker/
    └── Dockerfile.runner # mcr.microsoft.com/playwright:v1.40.0-jammy + python3
```

## Добавить новый сценарий

1. Добавь `Scenario(...)` в [api/scenarios.py](api/scenarios.py) — следующий sid.
2. Запусти `make e2e-staging-api` — новый тест появится автоматически (параметризация).
3. `scripts/seed-staging-companies.py --scenarios <new_sid>` создаст реальные данные.

## Расширение Playwright UI

Этапы 5-10 формы онбординга в UI пока не реализованы (бэкенд готов).
Когда появятся компоненты — добавить spec'и в `ui/specs/`:
- `applicant-step-5-le-profile.spec.ts`
- `applicant-step-6-activity.spec.ts`
- и т.д.

Покрытие на API-уровне уже есть через bff-onboarding GraphQL.

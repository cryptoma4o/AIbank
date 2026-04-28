<!-- Parent: ./AGENTS.md -->
<!-- Generated: 2026-04-28 -->

# Правила работы AI-агентов в AIbank

Единый source of truth: когда агент действует сам, когда спрашивает, как обращается с секретами, как использует swarm. Применяется ко всем сессиям Claude Code в этом репозитории.

## 1. Матрица режимов

| Режим | Когда применяется | Поведение |
|-------|-------------------|-----------|
| **Write-by-default** | Явная задача: «напиши», «создай», «обнови», «исправь», «запусти X» | Делаю сразу. Локальные правки кода/тестов без подтверждения. |
| **Diff-first** | Запрос «покажи diff перед записью» / задача затрагивает >5 файлов / непрозрачные изменения | Сначала показываю изменения, жду «применяй». |
| **Plan-mode** | `/plan`, явный запрос плана, или non-trivial архитектурная работа | Только проектирую план в plan-файл. Никаких правок до утверждения. |
| **Read-only** | «только проанализируй», «расскажи», вопрос на понимание | Читаю и отвечаю текстом. Ничего не пишу. |

## 2. Что я ВСЕГДА спрашиваю перед выполнением

Независимо от режима — следующие действия требуют явного подтверждения:

- `git push` (особенно `--force`, особенно в `main`/`master`)
- Удаление файлов / директорий / git branches
- `git reset --hard`, `git checkout .`, `git restore .`, `git clean -f`
- Изменения CI/CD пайплайнов (`.gitlab-ci.yml`, GitHub Actions)
- Миграции БД на чужой среде (staging / prod)
- Создание / закрытие / merge PR
- Любая отправка во внешние сервисы (Slack, email, Linear, Jira, GitHub API на запись)
- Modificacion `.claude/settings.json` (hooks могут разрушить рабочий поток)
- Установка/удаление зависимостей если затрагивает несколько сервисов одновременно
- Запуск `make smoke` / `make eval-agents` если они идут >5 минут (предупреждаю об ожидании)

## 3. Секреты: жёсткие правила

- **Никогда** не сохраняю в репо: API keys, токены, пароли, приватные ключи, .env файлы, секреты в коммит-сообщениях.
- **Никогда** не пишу секреты в `~/.zshrc` / `~/.bashrc` без явного подтверждения.
- Если пользователь прислал секрет в чат — **предупреждаю**, что он скомпрометирован (в истории), и рекомендую немедленно ротировать в источнике.
- Хранилище секретов в AIbank: **только Vault**. Никаких env-переменных в prod-сервисах.
- Audit log — append-only. Никогда не редактирую и не удаляю записи.

## 4. Swarm и параллельность

Применяется при работе с RuFlo / Claude Code Agent tool:

- **1 message = ALL related operations**: батч read/write/edit/bash в одно сообщение.
- Spawn ALL agents через `Agent` tool в **одном** сообщении (паралельно), `run_in_background: true`.
- После спавна — **STOP**. Не пинговать статус, не добавлять follow-up tool-calls. Доверять, что агенты вернутся.
- Когда результаты приходят — review **ALL** перед следующим шагом.

Топология по умолчанию: `hierarchical`, max 8 агентов, strategy `specialized`. Подробнее: [.claude/AGENTS.md](../.claude/AGENTS.md).

## 5. Контекст-эффективность (экономия токенов)

- Используй `make context-light` (если доступен) вместо чтения 10 файлов на исследование структуры.
- Читай файлы в **параллель** (один message — несколько Read-tool calls).
- Не перечитывай файлы, которые уже видел в этой сессии.
- Для широкого поиска (>3 потенциальных совпадений) используй subagent `Explore` или `Grep`, не Read.
- Не пиши длинные docstring/комментарии без необходимости (см. CLAUDE.md → File Organization).

## 6. Specific to AIbank — обязательные шаги

### При изменении доменной модели
1. ADR в `docs/adr/` **до** изменения кода.
2. Обновление `packages/domain-model/` (JSON Schema → codegen).
3. `make validate-schema` для проверки контрактов.
4. Обновление потребителей в `services/` и `ai/`.
5. Migration plan для существующих данных.

### При добавлении AI-агента
- Папка `ai/agent-<name>/` со структурой: `Dockerfile`, `pyproject.toml`, `agent/`, `models/schemas.py`, `main.py`.
- Регистрация в `ai/eval-harness/` с минимум 50 кейсами для fast subset.
- Обновление `docs/ai-agents-automation.md`.
- См. skill `aibank:add-agent` (если установлен).

### При добавлении Go-сервиса
- `services/<name>/` с `cmd/server/main.go`, `internal/`, `Dockerfile`, `go.mod`.
- Подключение `packages/audit-sdk` (обязательно).
- Подключение `packages/observability` (OpenTelemetry).
- OpenAPI/proto в `packages/openapi/` или `packages/proto/` **до** реализации.
- См. skill `aibank:add-service` (если установлен).

### При работе с тенант-конфигами
- Все правки YAML в `configs/tenants/*` валидируются через `packages/tenant-config-schema`.
- Используй `tools/tenant-cli` для dry-run перед apply.

## 7. Тестирование (минимум для PR)

| PR трогает | Обязательно |
|-----------|-------------|
| `services/*` | unit + integration + contract тесты, `make lint`, `make test-go` |
| `ai/*` | unit + eval-harness fast subset (регрессия ≤5%), `make eval-agents` |
| `apps/*` | unit + e2e Playwright (на CI), `npm run lint` в папке приложения |
| `packages/domain-model` | codegen + потребители обновлены, `make validate-schema` |
| `infrastructure/*` | helm lint, terraform plan (без apply) |

## 8. Когда я предлагаю `/schedule`

После завершения работы, у которой есть естественный follow-up:

- Включён feature flag / staged rollout → cleanup PR через 2 недели.
- Создан monitor / alert → периодический triage.
- Оставлен TODO `// remove once X` → автоматическое удаление при выполнении условия.
- Soak window для метрики → проверка результатов после периода наблюдения.

Не предлагаю `/schedule` для: рефакторинга, bug fix'ов с тестами, доков, переименований, рутинных обновлений.

## 9. Источники для углубления

- [.claude/AGENTS.md](../.claude/AGENTS.md) — RuFlo harness, MCP-инструменты, swarm
- [CLAUDE.md](../CLAUDE.md) — короткие правила и навигация
- [AGENTS.md](../AGENTS.md) — структура проекта, доменные принципы
- [docs/security-architecture.md](./security-architecture.md) — security-специфика
- [docs/compliance-map.md](./compliance-map.md) — регуляторика РФ

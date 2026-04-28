# Claude Code — AIbank

> **Точка входа для агентов**: [AGENTS.md](AGENTS.md) (структура, доменная модель, навигация по задаче).
> **Правила работы агентов**: [docs/agents-rules.md](docs/agents-rules.md) (когда писать сам, plan-mode, секреты).
> **Harness RuFlo / MCP**: [.claude/AGENTS.md](.claude/AGENTS.md) (swarm, memory, MCP-инструменты).

## Behavioral Rules (Always Enforced)

- Do what has been asked; nothing more, nothing less
- ALWAYS read a file before editing it
- ALWAYS prefer editing an existing file to creating a new one
- NEVER proactively create *.md / README files unless explicitly requested
- NEVER save working files, tests, or scratch notes to the repo root
- NEVER commit secrets, credentials, or .env files
- Не опрашивай статус swarm после спавна — дождись результатов

## File Organization (AIbank monorepo)

Никогда не клади файлы в корень. Используй существующую структуру:

| Директория | Назначение |
|-----------|-----------|
| `ai/` | AI-агенты (Python 3.12, FastAPI), llm-gateway, rag-service, eval-harness |
| `services/` | Go 1.22 микросервисы (chi, gRPC, sqlc, Temporal) |
| `apps/` | Next.js 14 фронтенды (web-onboarding, web-admin, …) |
| `packages/` | Shared библиотеки (Go/Python/TS): domain-model, proto, audit-sdk, ui-kit, … |
| `infrastructure/` | Helm, Terraform (Yandex Cloud), Ansible |
| `configs/` | YAML-конфиги тенантов |
| `tools/` | Внутренние утилиты: mock-abs, tenant-cli, data-generator |
| `docs/` | Документация (русский язык, см. `docs/AGENTS.md`) |
| `scripts/` | Автоматизация для разработки |

## Project Architecture (AIbank-specific)

- **Multi-tenant by design**: изоляция на уровне PostgreSQL schema, S3 bucket, Kafka topic. Никаких `tenant_id` в общих таблицах.
- **Hybrid deployment**: SaaS + on-prem. Запрещены managed-сервисы AWS/GCP — только PostgreSQL, Kafka, Redis, MinIO, Qdrant.
- **Event-driven**: Kafka — нервная система. Sync RPC только там, где критично для UX.
- **Configuration over code**: бизнес-правила и воркфлоу — в `configs/tenants/*.yaml`, не в коде.
- **Self-hosted LLM**: vLLM/SGLang в инфраструктуре банка, не за периметром.
- **Domain model = контракт**: изменения только через ADR (`docs/adr/`) + согласование архитектора.
- **DDD bounded contexts**: каждый сервис в `services/` — отдельный контекст. Файлы ≤500 строк. Типизированные интерфейсы для всех публичных API.
- **TDD London School** (mock-first) для нового кода.

## Build & Test

```bash
# Корневые таргеты
make build              # Сборка всех Docker-образов
make test               # test-go + test-python
make lint               # golangci-lint, ruff, eslint, spectral
make validate-schema    # Контракты: domain-model + tenant-config
make smoke              # E2E на docker-compose
make eval-agents        # Регрессия AI-агентов через eval-harness
make up / make down     # Локальный стек (postgres, kafka, redis, qdrant, minio, temporal)
```

- Для AI-PR обязательно `make eval-agents` — регрессия >5% блокирует merge.
- Для каждого ABS-адаптера — contract-тесты (golden tests с mock-ABS).

## Security Rules

- Никогда не хардкодь API keys / секреты / credentials в исходниках.
- Секреты — только в Vault, никогда в Git или env-переменных prod-сервисов.
- Валидируй пользовательский ввод на границах системы (api-gateway, BFF).
- Sanitize file paths (защита от directory traversal).
- Audit log — append-only, иммутабельный. Никогда не удаляй и не изменяй записи.
- Подробнее: [docs/security-architecture.md](docs/security-architecture.md), [docs/compliance-map.md](docs/compliance-map.md).

## Когда я выполняю сам, а когда спрашиваю

См. [docs/agents-rules.md](docs/agents-rules.md) — там полный матрица режимов (write-by-default / plan-mode / diff-first / read-only).

Кратко: опасные действия (push, force-push, удаление, миграции БД, отправка PR, отправка секретов наружу) — **всегда** с подтверждением. Локальные правки кода/тестов — без подтверждения, если задача явно поставлена.

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-28 -->

# tools/

## Purpose

Внутренние утилиты для разработки и тестирования: mock-АБС, CLI для управления тенантами, генератор синтетических данных, performance-бенчмарки. **Не деплоятся в production**. Используются локально и в CI.

## Текущие инструменты

| Директория | Назначение | Стек | Статус |
|-----------|-----------|------|--------|
| [`audit-verifier/`](audit-verifier/) | Валидация append-only audit-логов: проверка целостности, поиск пропусков, форматный аудит | Go | ready |
| [`benchmarks/`](benchmarks/) | k6-нагрузочные тесты для tenant/audit/llm-gateway/bff с baselines и result-сравнением | k6 + bash | ready |
| [`data-generator/`](data-generator/) | Синтетические кейсы онбординга: ИНН/ОГРН/паспорта с корректными контрольными суммами; PDF-уставы. Используется `ai/eval-harness/` | Python (Faker, ReportLab) | in-use (Ф0) |
| [`db-migrator/`](db-migrator/) | Apply/rollback миграций PostgreSQL для всех сервисов; вызывается через `make migrate-platform` | Go | ready |
| [`mock-abs/`](mock-abs/) | Единый mock ABS API для contract-тестов адаптеров (`abs-adapter-cft`, `abs-adapter-diasoft`, `abs-adapter-rs-bank`) | Go | ready |
| [`tenant-cli/`](tenant-cli/) | CLI управления тенантами: создание, обновление YAML-конфига, dry-run валидация против `packages/tenant-config-schema` | Go | ready |

## Запуск через Makefile

```bash
# Бенчмарки
make bench           # все k6-тесты + сравнение с baselines
make bench-seed      # пред-засев данных (5 тенантов, 250 audit events)
make bench-tenant    # tenant-service
make bench-audit     # audit-service
make bench-llm       # llm-gateway (mock mode)
make bench-bff       # bff-onboarding GraphQL

# Миграции
make migrate-platform  # одноразовый job через docker-compose
```

## For AI Agents

### Working In This Directory

- **mock-abs**: содержит ровно ту логику, что нужна для воспроизводимых golden tests. Не превращай в функциональный клон ABS — это нагрузит сопровождение без пользы.
- **data-generator**: синтетические данные **не должны** содержать реальные ПДн. ИНН/ОГРН генерируются с корректной контрольной суммой по алгоритму ФНС. Seed фиксированный для воспроизводимости.
- **tenant-cli**: только CLI-обёртка. Логика валидации — в `packages/tenant-config-schema/`.
- **audit-verifier**: пишет результаты в structured-формате (JSON), легко интегрируется в CI и тенант-инструментарий.
- **benchmarks**: baselines в `tools/benchmarks/baselines/` под git, обновлять их — отдельный PR с обоснованием.

### Testing Requirements

- mock-abs: тестируется вместе с потребителями (contract tests в `services/abs-adapter-*/`)
- data-generator: smoke-tests на корректность форматов (ИНН checksum, ОГРН checksum, валидный PDF)
- tenant-cli: integration tests с тестовым ArgoCD endpoint
- db-migrator: up/down rollback тестируется в `make smoke`
- audit-verifier: golden-fixtures для каждого формата записи

### Common Patterns

Структура Go-инструмента (mock-abs / tenant-cli / db-migrator / audit-verifier):
```
tool-name/
├── Dockerfile
├── cmd/<binary>/main.go
├── internal/                # бизнес-логика
├── README.md                # доки по флагам/эндпоинтам
└── go.mod / go.sum
```

CLI-инструменты следуют принципу UNIX: одна команда — одна задача, выход 0 = success, ошибки в stderr.

## Что планируется (нет ещё)

- **mock-abs-cft / mock-abs-diasoft** как отдельные сервисы — пока единый `mock-abs/` покрывает три адаптера. Разделение, если понадобится разная логика для каждого ABS, — через отдельный ADR.

## Dependencies

### Internal
- `../packages/domain-model/` — типы для data-generator
- `../packages/tenant-config-schema/` — валидация в tenant-cli
- `../services/abs-adapter-*/` — потребители mock-abs
- `../ai/eval-harness/` — потребитель data-generator

### External
- Faker (Python), ReportLab — data-generator
- k6 — benchmarks
- golang-migrate — db-migrator

<!-- MANUAL: -->

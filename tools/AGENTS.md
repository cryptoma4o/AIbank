<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# tools/

## Purpose

Внутренние утилиты для разработки и тестирования: mock-реализации АБС, CLI для управления тенантами, генератор синтетических данных. Не деплоятся в production. Статус: **директория пустая, инструменты создаются с Фазы 2**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор внутренних инструментов: mock-ABS, CLI, генератор данных для eval-harness |

## Planned Subdirectories

| Директория | Назначение | Фаза |
|-----------|-----------|------|
| `mock-abs-cft/` | Mock ЦФТ для contract-тестов `abs-adapter-cft/`. Реализует ABS API по golden fixtures. Go | Ф2 |
| `mock-abs-diasoft/` | Mock Diasoft FA# для contract-тестов `abs-adapter-diasoft/`. Go | Ф2 |
| `tenant-cli/` | CLI для управления тенантами: создание, обновление конфигов, dry-run валидация, применение через ArgoCD. Go | Ф1 |
| `data-generator/` | Генератор синтетических кейсов онбординга: паспорта, уставы (PDF), структуры владения, ЕГРЮЛ-выписки. Используется для eval-harness. Python | Ф0 М0 |

## For AI Agents

### Working In This Directory

- Mock-АБС **не должны** содержать логику, кроме минимально необходимой для golden tests. Цель — воспроизводимые тесты, не функциональный клон.
- `data-generator/` критичен для Фазы 0: нужны синтетические данные для `ai/eval-harness/datasets/docs-parsing/` и `ai/eval-harness/datasets/rag-quality/` до появления реальных данных.
- Синтетические данные **не должны** содержать реальные ПДн. Генерировать полностью фиктивные паспортные данные, ИНН (с корректной контрольной суммой), ОГРН.
- `tenant-cli/` — только CLI-интерфейс. Логика валидации конфигов — в `packages/tenant-config-schema/`.

### Testing Requirements

- Mock-АБС: тестируются вместе с соответствующими адаптерами (contract tests в `services/abs-adapter-*/`)
- `data-generator/`: smoke tests на корректность форматов (ИНН checksum, ОГРН checksum, валидный PDF)
- `tenant-cli/`: integration тесты с тестовым ArgoCD endpoint

### Common Patterns

Структура mock-ABS:
```
mock-abs-cft/
├── cmd/server/main.go
├── fixtures/             # golden request/response пары
├── handlers/             # HTTP handlers имитирующие ABS API
└── README.md             # описание поддерживаемых эндпоинтов
```

Алгоритм генерации ИНН (10 знаков для юрлиц, 12 для физлиц) должен проходить проверку контрольных сумм по алгоритму ФНС.

Mock-данные генерируются детерминированно (фиксированный seed для воспроизводимости тестов). CLI-инструменты следуют принципу UNIX: одна команда — одна задача.

## Dependencies

### Internal
- `../packages/domain-model/` — типы для data-generator
- `../packages/tenant-config-schema/` — схемы для tenant-cli валидации
- `../services/abs-adapter-*/` — потребители mock-ABS

### External
- Faker (Python) или аналог для генерации базовых данных
- ReportLab или fpdf2 (Python) для генерации PDF-документов

<!-- MANUAL: -->

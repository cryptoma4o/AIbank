<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# docs/adr/

## Purpose

Architecture Decision Records — краткие документы (1–3 страницы), фиксирующие значимые архитектурные решения, их контекст, рассмотренные альтернативы и последствия. ADR неизменяемы после принятия: если решение меняется — пишется новый ADR со статусом `Superseded by ADR-NNNN`.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Правила ведения ADR, когда писать/не писать, процесс согласования, список всех ADR |
| `0000-template.md` | Шаблон нового ADR — копировать при создании |
| `0001-use-temporal-for-orchestration.md` | **Accepted.** Выбор Temporal вместо самописного state machine / Step Functions / Airflow / Camunda для оркестрации онбординг-воркфлоу |

## Pending ADR (Proposed, не приняты)

| Номер | Тема |
|-------|------|
| 0002 | Schema-per-tenant vs database-per-tenant в PostgreSQL |
| 0003 | GraphQL Federation vs REST для BFF |
| 0004 | Nx vs Bazel для монорепо |
| 0005 | Стратегия миграций БД между тенантами |
| 0006 | Версионирование ABS-адаптеров |
| 0007 | Промпт-менеджмент: in-code vs external service |
| 0008 | Vector DB: Qdrant vs pgvector |
| 0009 | Стратегия on-prem обновлений (rolling vs blue-green) |
| 0010 | Биллинг-модель и audit log |
| 0011 | Стратегия выбора и роутинга LLM (multi-model pool) |
| 0012 | Eval-corpus governance |

## For AI Agents

### Working In This Directory

- Перед добавлением новой зависимости или смены технологии — проверь список ADR: решение может быть уже принято.
- Для создания нового ADR: скопируй `0000-template.md` → `NNNN-kebab-case-title.md`, заполни, открой PR со статусом `Proposed`.
- ADR принимается после аппрува минимум двух ревьюверов (один из смежной команды) — смени статус на `Accepted`.
- Никогда не редактируй содержимое принятых ADR — только статус (если superseded).
- Обновляй таблицу в `README.md` при добавлении нового ADR.

**Когда НЕ писать ADR:** тактические решения уровня одной задачи; фиксация факта (а не решения); решения, которые легко изменить (например, выбор UI-библиотеки внутри одного компонента).

### Common Patterns

Структура ADR:
```
# ADR-NNNN: Заголовок
Status: Proposed | Accepted | Superseded by ADR-XXXX
Date: YYYY-MM-DD
Authors: ...
Reviewers: ...

## Context
## Decision
## Alternatives Considered
## Consequences
## Implementation Notes
## References
```

## Dependencies

### Internal
- `../technical-structure.md` — раздел 13 содержит полный список решений, требующих ADR
- `../domain-model.md` — изменения в доменной модели требуют ADR при breaking changes

<!-- MANUAL: -->

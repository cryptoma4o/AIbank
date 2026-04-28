<!-- Parent: ./README.md -->

# Runbook: восстановление событий из outbox DLQ

## Когда применять

Когда в `*_outbox_dead_letter` таблице появились события, требующие пересмотра/переотправки.
Триггеры:
- Алерт `outbox_dlq_rate_high` (когда настроим метрики)
- Жалоба от потребителя «не вижу события N»
- Регулярная проверка on-call

## Контракт DLQ

Для каждого сервиса с outbox-таблицей `<schema>.<service>_outbox` есть DLQ-таблица
`<schema>.<service>_outbox_dead_letter` с одинаковой структурой + `failed_at`, `attempts`, `last_error`.

Текущие конфигурации:

| Сервис | Outbox-таблица | DLQ-таблица |
|--------|----------------|-------------|
| billing-service | `platform.billing_outbox` | `platform.billing_outbox_dead_letter` |

`MaxAttempts` по умолчанию = 5 (см. `packages/outbox/outbox.go` const `DefaultMaxAttempts`).

## Шаг 1. Снять inventory DLQ

```sql
-- Сколько и за какой период
SELECT COUNT(*) AS total,
       MIN(failed_at) AS oldest,
       MAX(failed_at) AS newest,
       COUNT(DISTINCT last_error) AS distinct_errors
FROM platform.billing_outbox_dead_letter;

-- Топ-5 типов ошибок
SELECT last_error, COUNT(*) AS n
FROM platform.billing_outbox_dead_letter
GROUP BY last_error
ORDER BY n DESC
LIMIT 5;

-- Распределение по event_type — что чаще всего падает
SELECT event_type, COUNT(*) AS n
FROM platform.billing_outbox_dead_letter
GROUP BY event_type
ORDER BY n DESC;
```

## Шаг 2. Классификация причин

| Класс | Признак | Что делать |
|-------|---------|------------|
| **Transient (Kafka outage)** | Похожие `last_error` про сеть/timeout, узкий период `failed_at` | Replay → шаг 3 |
| **Schema mismatch** | `last_error` про validation/parse, конкретный `event_type` | Исправить consumer/schema → деплой → replay → шаг 3 |
| **Poisoned payload** | `last_error` про malformed JSON, разные timestamps | Анализ payload, фикс источника → шаг 4 (selective replay/discard) |
| **Топик удалён / переименован** | `last_error` про unknown topic | Создать топик, обновить relay-конфиг → replay → шаг 3 |

## Шаг 3. Bulk replay (вернуть всё в работу)

**Опция A — через прямой INSERT обратно** (рекомендуется для Pre-MVP, простая):

```sql
BEGIN;

-- Переносим в outbox с обнулённым attempts (relay подберёт на ближайшем тике).
INSERT INTO platform.billing_outbox
    (id, aggregate_type, aggregate_id, event_type, payload, created_at, published_at, attempts, last_error, last_attempt_at)
SELECT id, aggregate_type, aggregate_id, event_type, payload, created_at, NULL, 0, NULL, NULL
FROM platform.billing_outbox_dead_letter
WHERE failed_at >= '2026-04-28 10:00:00';   -- фильтр по периоду transient outage

-- Удаляем перенесённые из DLQ.
DELETE FROM platform.billing_outbox_dead_letter
WHERE failed_at >= '2026-04-28 10:00:00';

-- Сохраняем audit-trail в comment перед коммитом.
COMMENT ON TABLE platform.billing_outbox_dead_letter IS
    'Replayed N rows from DLQ at 2026-04-28T11:00 by oncall (incident INC-XXX)';

COMMIT;
```

> ⚠️ **Перед COMMIT** — обязательно `SELECT COUNT(*)` с тем же WHERE и убедиться, что число совпадает с ожидаемым.

**Опция B — через restart (если нет времени на ручной перенос)**:

После исправления root-cause просто рестартани сервис — relay подберёт всё новое из outbox. Этот способ **не вытянет** DLQ-строки автоматически (они уже не в outbox).

## Шаг 4. Selective replay / discard

Когда часть DLQ — это poisoned messages, которые нельзя replay'ить, а часть — нормальные.

```sql
-- Posмотрим конкретное сообщение
SELECT id, aggregate_id, event_type, payload, last_error
FROM platform.billing_outbox_dead_letter
WHERE id = 12345;

-- Если poisoned — discard с фиксацией в audit log
INSERT INTO platform.audit_events (event_type, ref_id, summary, created_at)
VALUES ('outbox_dlq_discarded', '12345', 'Discarded poison message: malformed UUID', NOW());

DELETE FROM platform.billing_outbox_dead_letter WHERE id = 12345;
```

## Шаг 5. Превентивные меры

После recovery — обновить:

1. **CHANGELOG.md** — что чинили, что было причиной
2. **Postmortem** — если выкатили транзиент в prod из-за DLQ-блокера
3. **Метрики** — добавить алерт на DLQ-rate, если ещё нет
4. **Schema tests** — если poison был из-за schema drift, добавить contract-тест

## Связанные документы

- ADR-0010 § 2: outbox pattern
- [packages/outbox/README.md](../../packages/outbox/README.md) — техническая документация
- [docs/runbooks/incident-response.md](./incident-response.md) — общий incident workflow
- [docs/runbooks/audit-log-integrity.md](./audit-log-integrity.md) — для discard-операций

# Runbooks

Операционные runbook'и для on-call инженеров и DevOps платформы AIbank. Открываются в 3 часа ночи во время инцидента — оптимизированы под скорость, не под академичность.

## Когда использовать

- Получили P0/P1 алерт — открыть [`incident-response.md`](incident-response.md) для триажа
- Нужно применить миграции БД (или они упали) — [`db-migration.md`](db-migration.md)
- Temporal-воркфлоу зависли — [`temporal-recovery.md`](temporal-recovery.md)
- Чат-агент или OCR не отвечает — [`llm-gateway-troubleshooting.md`](llm-gateway-troubleshooting.md)
- Нашему инженеру нужен экстренный доступ к prod-данным банка — [`break-glass-procedure.md`](break-glass-procedure.md)
- Подозрение на манипуляцию с audit log — [`audit-log-integrity.md`](audit-log-integrity.md)

## Каталог

| Runbook | Когда читать | SLA реакции |
|---|---|---|
| [`incident-response.md`](incident-response.md) | Любой инцидент | Critical: 15 мин, High: 1 час |
| [`db-migration.md`](db-migration.md) | Релиз с DDL-изменениями, ошибка `db-migrator` | До и во время релиза |
| [`temporal-recovery.md`](temporal-recovery.md) | Алерт `workflow.stuck`, `temporal.cluster.unhealthy` | 1 час |
| [`llm-gateway-troubleshooting.md`](llm-gateway-troubleshooting.md) | LLM-агенты возвращают 5xx, `gateway.error_rate > 5%` | 30 мин |
| [`break-glass-procedure.md`](break-glass-procedure.md) | Недоступ к prod-данным мешает разрешить инцидент | По запросу |
| [`audit-log-integrity.md`](audit-log-integrity.md) | Алерт `audit.hash_chain_broken` | Critical (немедленно) |

## Деплой и эксплуатация (соседние документы)

- [`../deployment/README.md`](../deployment/README.md) — установка SaaS / on-prem, формирование bundle
- [`../operations/README.md`](../operations/README.md) — backup, мониторинг, onboarding инженера

## Ссылки на ADR

- [ADR-0001](../adr/0001-use-temporal-for-orchestration.md) — Temporal-оркестрация
- [ADR-0005](../adr/0005-db-migrations-strategy.md) — миграции БД
- [ADR-0009](../adr/0009-on-prem-update-strategy.md) — blue-green обновления on-prem
- [ADR-0010](../adr/0010-billing-and-audit-log.md) — audit log + биллинг
- [ADR-0011](../adr/0011-llm-routing-strategy.md) — LLM роутинг

## Соглашения

- Все runbook'и начинаются с раздела «Когда использовать»
- Команды копипастятся напрямую (после подстановки `<placeholders>`)
- Тенант-плейсхолдеры — `<tenant>` (например, `bank-alpha`)
- Сервис-плейсхолдеры — `<service>` (например, `tenant-service`)
- В случае расхождения runbook ↔ ADR — приоритет у ADR; обновить runbook через PR

## Owner

DevOps-лид + Security-инженер. PR — обязательный review хотя бы одного из них.

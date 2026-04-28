# Operations Documentation

Эксплуатационная документация для DevOps и SRE: backup, мониторинг, onboarding инженеров.

## Когда использовать

- Настраиваете backup для нового кластера
- Проверяете DR-готовность (quarterly drill)
- Нужно понять SLO / SLI / алерт-структуру
- Onboarding нового инженера в команду

## Каталог

| Документ | Когда читать |
|---|---|
| [`backup-restore.md`](backup-restore.md) | Настройка backup, DR, восстановление stateful-сервисов |
| [`monitoring-alerts.md`](monitoring-alerts.md) | LGTM stack, SLO, alert rules, dashboards |
| [`onboarding-engineer.md`](onboarding-engineer.md) | Первый день нового инженера |

## Ключевые SLO платформы

(Подробности в [`monitoring-alerts.md`](monitoring-alerts.md))

| SLI | SLO | Категория алерта при нарушении |
|---|---|---|
| Доступность платформы (any tenant) | 99.9% | P0 |
| p95 latency онбординг-флоу | <30s | P1 |
| Error rate per tenant | <0.5% | P1 |
| Audit log ingest lag | <60s | P0 |
| RPO (Recovery Point Objective) | 15 мин | DR-метрика |
| RTO (Recovery Time Objective) | 4 часа | DR-метрика |

## DR посетить раз в квартал

DR drill (per `security-architecture.md` § 11) — обязательная квартальная активность.
Чеклист — в [`backup-restore.md`](backup-restore.md) § «Quarterly DR drill checklist».

## Связанные документы

- [`../runbooks/README.md`](../runbooks/README.md) — операционные runbook'и для инцидентов
- [`../deployment/README.md`](../deployment/README.md) — установка платформы
- [`../security-architecture.md` § 9.3](../security-architecture.md) — требования к DR

## Owner

DevOps-лид + SRE. Backup-стратегия согласована с DBA и Security-инженером.

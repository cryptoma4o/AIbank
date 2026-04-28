# Incident Response

## Когда использовать

Любая алертовая ситуация P0/P1, заявка на security-инцидент, сообщение от банка-клиента «у нас что-то не работает», или подозрение на компрометацию. Подразделяется на категории — см. ниже.

Ссылки: [`security-architecture.md` § 9](../security-architecture.md), [ADR-0010 § «Audit log»](../adr/0010-billing-and-audit-log.md).

## Категории и SLA

Категории и SLA взяты из `security-architecture.md` § 9.1.

| Категория | Примеры | SLA уведомления банку | SLA уведомления РКН |
|---|---|---|---|
| **Critical (P0)** | Утечка ПДн, компрометация ключей, неавторизованный доступ к prod | 1 час | 24 часа (152-ФЗ) |
| **High (P1)** | Подозрение на инцидент, обход одного слоя защиты | 4 часа | По решению DPO |
| **Medium (P2)** | Уязвимость без эксплуатации, misconfiguration | 1 рабочий день | — |
| **Low** | Гигиенические находки | Еженедельный отчёт | — |

## Алгоритм триажа

```
Detect → Triage → Contain → Investigate → Remediate → Notify → Post-mortem
```

| Шаг | Что делать | Кто owner | Артефакт |
|---|---|---|---|
| Detect | Алерт из VictoriaMetrics/Loki/SIEM, отчёт пользователя, CERT-уведомление | On-call | Тикет в incident-tracker |
| Triage | Оценить категорию, эскалация security-офицеру при подозрении на P0 | On-call → security-офицер | Категория + initial impact |
| Contain | Изоляция: revoke key, block IP, scale-to-zero сервис, отключить tenant | On-call + DBA | Команды записаны в тикет |
| Investigate | Forensics: логи, audit-events, трассы, snapshot диска | Security-инженер | Timeline |
| Remediate | Forward fix, hotfix-релиз, ротация ключей | Tech lead | PR + deploy ID |
| Notify | Банк по согласованному каналу + DPO + (при необходимости) РКН | Security-офицер | Письмо банку |
| Post-mortem | Документ за 5 рабочих дней (P0/P1 — обязательно) | Owner инцидента | docs/postmortems/YYYY-MM-DD-... |

## Контакты (placeholders — заполнить перед go-live)

| Роль | Канал | SLA ответа | Owner |
|---|---|---|---|
| Наш security-officer (24/7) | Mattermost `#sec-oncall`, телефон | 15 мин | TBD |
| Наш CTO / эскалация P0 | Telegram, телефон | 30 мин | TBD |
| DPO банка-клиента | Per-tenant в `configs/tenants/<tenant>/contacts.yaml` | 1 час | Banking |
| CISO банка-клиента | Per-tenant | 4 часа | Banking |
| РКН (152-ФЗ нотификация) | Через DPO банка, [pd.rkn.gov.ru](https://pd.rkn.gov.ru/) | 24 часа от обнаружения | DPO |
| ЦБ ФинЦЕРТ | `incident@cbr.ru`, форма ФинЦЕРТа | 24 часа для КИИ | Banking CISO |
| On-call rotation | PagerDuty `aibank-platform` (TBD) | 5 мин | DevOps |

## Топ-5 типовых инцидентов

### Инцидент 1: Утечка ПДн (Critical)

**Признаки:** аномалия в access-логах (массовый export-запрос, незнакомый IP), внешний отчёт о публикации БД.

```bash
# 1. CONTAIN — заблокировать актора и отозвать его токены
kubectl exec -n platform deploy/identity-service -- \
  identity-cli session revoke --user=<user_id> --reason=incident-<id>

# 2. CONTAIN — ротировать tenant key per ADR-0010 (§ Audit Log)
kubectl exec -n platform deploy/tenant-service -- \
  tenant-cli rotate-key --tenant=<tenant> --reason="P0 leak <id>"

# 3. INVESTIGATE — сохранить audit-цепочку для tenant за период
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "COPY (SELECT * FROM tnt_<tenant>.audit_events
         WHERE recorded_at BETWEEN '<from>' AND '<to>')
   TO '/secure-snap/audit-<id>.csv' WITH CSV HEADER"

# 4. INVESTIGATE — cross-check с access-логами
kubectl logs -n platform deploy/api-gateway --since=24h | \
  grep "<tenant>" | grep -E "GET /v1/(applications|clients)" > /tmp/access-<id>.log

# 5. NOTIFY — банк через 1 час, DPO банка → подаёт в РКН в 24 часа
# Шаблон письма: docs/runbooks/templates/incident-notification.md (TBD)
```

**Обязательно:**
- Audit-events НЕ удаляются (immutable per ADR-0010)
- Сохранить snapshot БД в forensic-окружение до remediation
- РКН-уведомление через DPO банка в 24 часа от **обнаружения**, не от инцидента

### Инцидент 2: Failed pod restart loop в проде

**Признаки:** алерт `pod.crash_loop`, `RestartCount > 5 за 10 мин`.

```bash
# 1. Посмотреть pod и события
kubectl describe pod -n <namespace> <pod-name>
kubectl get events -n <namespace> --sort-by='.lastTimestamp' | tail -30

# 2. Логи последнего рестарта и предыдущего
kubectl logs -n <namespace> <pod-name> --tail=200
kubectl logs -n <namespace> <pod-name> --tail=200 --previous

# 3. Проверить, не миграции ли это (наш самый частый случай)
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT tenant_id, service_name, MAX(version) AS applied
   FROM platform.tenant_migrations
   WHERE service_name = '<service>'
   GROUP BY tenant_id, service_name"
# Сравнить с ожидаемой версией из релиза

# 4. Проверить readiness/liveness probe URL
kubectl exec -n <namespace> <pod-name> -- wget -qO- http://localhost:8080/health

# 5. ConfigMap / Secret refresh? Что менялось в последний час
kubectl rollout history deploy/<service> -n <namespace>
```

**Decision tree:**
- `OOMKilled` → увеличить `resources.limits.memory`, проверить утечку
- `CrashLoopBackOff` + DB connection error → проверить миграции и БД health
- `ImagePullBackOff` → проверить, что image push прошёл (см. CI pipeline)
- Если не починилось за 30 мин → блю-грин rollback per ADR-0009

### Инцидент 3: Temporal worker hang

**Признаки:** алерт `workflow.stuck` (Workflow в state Running > expected duration), очередь Activity растёт.

```bash
# 1. Подключиться к Temporal CLI (через port-forward или из bastion-pod)
kubectl port-forward -n platform-data svc/temporal-frontend 7233:7233 &
export TEMPORAL_ADDRESS=localhost:7233

# 2. Описание проблемного workflow
tctl workflow describe --workflow_id <wf_id>
tctl workflow showid <wf_id> --output_filename /tmp/wf-<id>.history

# 3. Список pending activities на task queue
tctl taskqueue describe --taskqueue onboarding-default

# 4. Если workflow «безнадёжен» (deadlock в коде, а не внешний сбой):
tctl workflow terminate --workflow_id <wf_id> --reason "stuck: <root-cause>"

# 5. Если worker сам hung — рестарт
kubectl rollout restart deploy/onboarding-orchestrator -n platform
```

Подробности в [`temporal-recovery.md`](temporal-recovery.md).

### Инцидент 4: PostgreSQL replication lag > 30s

**Признаки:** алерт `pg.replication.lag.seconds > 30`.

```bash
# 1. Проверить статус репликации на primary
PGPASSWORD=$DBA_PWD psql -h <primary-host> -U dba -d aibank -c \
  "SELECT client_addr, state, sync_state,
          pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn)/1024/1024 AS lag_mb,
          replay_lag
   FROM pg_stat_replication"

# 2. На реплике — посмотреть, что она отстаёт от
PGPASSWORD=$DBA_PWD psql -h <replica-host> -U dba -d aibank -c \
  "SELECT pg_is_in_recovery(), pg_last_wal_replay_lsn(), pg_last_xact_replay_timestamp()"

# 3. Diagnose: сеть / WAL volume / locks
kubectl top pod -n platform-data -l app=postgres
kubectl exec -n platform-data <primary-pod> -- iostat 2 5

# 4. Decision tree:
#    - lag растёт линейно + сетевая утилизация низкая → реплика не справляется (CPU/IO)
#    - lag скачкообразно > 5min + primary под нагрузкой → массовый INSERT/UPDATE (миграция?)
#    - lag > 5 минут + RTO задышит → решать про failover
```

**Failover decision:**
- < 1 минуты lag — мониторим
- 1–5 минут — PagerDuty wake the DBA, не failover'имся
- 5+ минут И primary недоступен → failover (только DBA + on-call lead, никогда соло)
- Failover-procedure: [`../operations/backup-restore.md` § «Postgres failover»](../operations/backup-restore.md)

### Инцидент 5: Внешний API недоступен (ЕГРЮЛ / ЕСИА / Росфинмониторинг / ФССП)

**Признаки:** `ext-egrul.error_rate > 50%`, `ext-esia.timeout > 10s p95`.

```bash
# 1. Подтвердить, что это внешняя проблема, а не наша
curl -fsS https://egrul.nalog.ru/  # упрощённый ping
kubectl exec -n platform deploy/ext-egrul -- env | grep -E "(EGRUL_|HTTP_PROXY)"

# 2. Включить degraded mode (cache-only)
kubectl set env -n platform deploy/ext-egrul EGRUL_CACHE_ONLY=true
kubectl rollout status deploy/ext-egrul -n platform

# 3. Найти заявки в проблемном статусе
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT tenant_id, count(*)
   FROM tnt_<tenant>.applications
   WHERE state IN ('waiting_egrul', 'egrul_check_failed')
     AND updated_at > now() - interval '4 hours'
   GROUP BY tenant_id"

# 4. Уведомить affected тенантов через notification-service
# (письмо: «ЕГРЮЛ временно недоступен; заявки автоматически переподтянутся через 1 час»)

# 5. Когда ЕГРЮЛ вернулся — запустить retry workflow
tctl workflow start \
  --taskqueue ext-retry \
  --workflow_type ExtRetryAllPending \
  --input '{"service":"ext-egrul","since":"<timestamp>"}'

# 6. Выключить degraded mode
kubectl set env -n platform deploy/ext-egrul EGRUL_CACHE_ONLY-
```

## Шаблон Post-mortem

Файл: `docs/postmortems/YYYY-MM-DD-<short-name>.md`. Обязателен для P0/P1.

```markdown
# Post-mortem: <название инцидента>

- **Date:** YYYY-MM-DD
- **Duration:** HH:MM МСК — HH:MM МСК (X часов Y минут)
- **Severity:** P0 | P1
- **Owner:** <имя>
- **Affected tenants:** <список>
- **Customer impact:** <человеко-читаемо: «5 банков N часов не могли создавать заявки на ИП»>

## Timeline (МСК)

- HH:MM — алерт `...` сработал в Mattermost #sec-oncall
- HH:MM — on-call принял
- HH:MM — эскалация в security-офицер
- HH:MM — обнаружена root cause: ...
- HH:MM — применили mitigation: ...
- HH:MM — система вернулась в норму
- HH:MM — банк уведомлён (письмо <id>)
- HH:MM — public statement (если применимо)

## Root cause

<техническое описание; 1-2 параграфа>

## Что сработало

- ...

## Что не сработало

- ...

## Action items

| # | Что | Owner | Due | Tracker |
|---|---|---|---|---|
| 1 | ... | <name> | YYYY-MM-DD | JIRA-XXX |

## Уроки

<2-5 пунктов: что мы узнали о системе и как изменим процесс>
```

## Что обязательно для P0

- [ ] Открыт incident-тикет в течение 15 мин
- [ ] Уведомлён security-офицер
- [ ] Уведомлён CISO банка в течение 1 часа
- [ ] Audit-events за период инцидента сохранены в forensic-snapshot
- [ ] DPO банка проинформирован → решение про РКН в 24 часа
- [ ] Post-mortem опубликован в течение 5 рабочих дней
- [ ] Все action items внесены в трекер с owner и due-date

## Связанные документы

- [`security-architecture.md` § 9](../security-architecture.md) — incident management
- [`break-glass-procedure.md`](break-glass-procedure.md) — если нужен экстренный доступ
- [`audit-log-integrity.md`](audit-log-integrity.md) — при подозрении на tampering
- [`temporal-recovery.md`](temporal-recovery.md), [`db-migration.md`](db-migration.md) — узкие классы инцидентов
- [ADR-0010](../adr/0010-billing-and-audit-log.md) — audit-events для notify-цепочки

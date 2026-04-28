# Audit Log Integrity

## Когда использовать

- Алерт `audit.hash_chain_broken` (Critical, P0)
- Подозрение на manual UPDATE/DELETE в `audit_events`
- Регулярная проверка целостности (раз в неделю)
- Регуляторный аудит банка спрашивает «как вы доказываете неизменность audit log»
- После любого break-glass — обязательно

Ссылки: [`security-architecture.md` § 8.3](../security-architecture.md), [ADR-0010](../adr/0010-billing-and-audit-log.md).

## Структура audit log

```
tnt_<tenant>.audit_events
├── id              UUID PK
├── recorded_at     TIMESTAMPTZ
├── actor           JSONB        -- {type, id}
├── action          TEXT         -- 'account.opened', 'decision.approved', ...
├── subject         JSONB        -- {type, id}
├── data            JSONB        -- payload
├── prev_hash       TEXT         -- SHA-256 предыдущей записи
├── current_hash    TEXT         -- SHA-256 текущей записи (включая prev_hash)
├── chain_position  BIGINT       -- порядковый номер
├── signature       TEXT         -- подпись Ed25519 / ГОСТ
└── tenant_id       TEXT
```

Hash-цепочка:
```
current_hash = SHA-256(
  id || recorded_at || actor || action || subject || data || prev_hash
)
```

Любая модификация любой записи делает её `current_hash` невалидным → ломается chain для всех последующих записей.

Append-only enforced:
- Триггер `audit_events_immutable` запрещает UPDATE/DELETE на уровне БД
- Роли приложений (`audit_app`) имеют только INSERT
- DELETE возможен только под superuser → audit-event `platform.audit_deleted` (но это уже сигнал tampering)

## Tool для проверки цепочки

В `tools/audit-verifier/` (deferred — TBD имплементировать в Phase 2):

```bash
# Полная проверка одного тенанта
audit-verifier verify --tenant=<tenant> --dsn="postgres://..."

# Output (success):
#   tenant: bank-alpha
#   events: 192410
#   chain: VALID
#   signatures: VALID (192410/192410)
#   first: 2026-04-01T00:00:01Z
#   last:  2026-04-26T15:32:18Z

# Output (failure):
#   tenant: bank-alpha
#   chain: BROKEN at chain_position=12345
#   expected prev_hash=abc123...
#   actual   prev_hash=def456...
#   ALERT: tampering suspected

# Точечная проверка (диапазон)
audit-verifier verify --tenant=<tenant> --from-position=10000 --to-position=20000

# Проверка одной записи
audit-verifier inspect --tenant=<tenant> --event-id=<uuid>
```

До появления tool — ручная SQL-проверка:

```sql
-- Найти разрывы цепочки
WITH ordered AS (
  SELECT id, recorded_at, prev_hash, current_hash, chain_position,
         LAG(current_hash) OVER (ORDER BY chain_position) AS expected_prev
  FROM tnt_<tenant>.audit_events
)
SELECT chain_position, id, prev_hash, expected_prev
FROM ordered
WHERE chain_position > 1
  AND prev_hash IS DISTINCT FROM expected_prev;

-- Проверить, что нет дыр в позициях
SELECT chain_position, chain_position - LAG(chain_position) OVER (ORDER BY chain_position) AS gap
FROM tnt_<tenant>.audit_events
WHERE chain_position - LAG(chain_position) OVER (ORDER BY chain_position) > 1;

-- Подсчёт по action — sanity check
SELECT action, count(*) AS n,
       MIN(recorded_at) AS first_at, MAX(recorded_at) AS last_at
FROM tnt_<tenant>.audit_events
GROUP BY action
ORDER BY n DESC;
```

## Что делать при разрыве цепочки (TAMPERING ALERT)

**Это всегда P0 инцидент.** См. [`incident-response.md` § «Утечка ПДн»](incident-response.md) — параллельный класс.

```bash
# 1. CONTAIN — заморозить запись в audit для tenant'а на анализ
#    (НЕ удалить ничего из audit, наоборот — снять snapshot)
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "COPY (SELECT * FROM tnt_<tenant>.audit_events ORDER BY chain_position)
   TO '/secure-snap/audit-<tenant>-<incident_id>.csv' WITH CSV HEADER"

# 2. Снять PG-снэпшот WAL для forensic
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT pg_create_physical_replication_slot('forensic_<incident_id>')"
# WAL сохраняется до явного drop; даёт «откат» к моменту обнаружения

# 3. INVESTIGATE — найти точку разрыва
audit-verifier verify --tenant=<tenant> --quick > /tmp/audit-<id>.report

# 4. INVESTIGATE — кто модифицировал
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT * FROM pg_stat_activity WHERE datname='aibank' AND query LIKE '%audit_events%'"
# pg_audit logs (если включён pgAudit extension) дадут историю UPDATE/DELETE-попыток

# 5. INVESTIGATE — есть ли break-glass-сессии в этот период
PGPASSWORD=$DBA_PWD psql -h <pg-host> -U dba -d aibank -c \
  "SELECT * FROM platform.audit_events
   WHERE action LIKE 'platform.break_glass_%'
     AND recorded_at BETWEEN '<2 hours before>' AND '<incident time>'"

# 6. CONTAIN — отзвонить ВСЕ admin-credentials
kubectl exec -n platform deploy/identity-service -- \
  identity-cli session revoke-all --role=platform.admin --reason=incident-<id>

# 7. NOTIFY — банк в течение 1 часа (Critical SLA)
#    DPO + CISO + Compliance officer
#    Приложить: точка разрыва, временной диапазон, scope (какие действия могли быть подделаны)

# 8. REMEDIATE — нельзя «починить» цепочку (это будет ещё одна подделка)
#    Создаётся новый audit-стрим:
#      tnt_<tenant>.audit_events_v2 — новая таблица, новая цепочка
#      audit-event platform.audit_chain_compromised записан как «закладка»
#      Все клиенты audit-sdk пишут в v2 (через config flip)

# 9. Forensics handover в банк
#    - CSV-снэпшот audit
#    - WAL-снэпшот за период
#    - Отчёт audit-verifier
#    - Список admin-сессий за период
#    - Видеозаписи break-glass за период (если были)

# 10. Post-mortem обязательно (P0)
```

**Ничего не удалять из audit.** Даже если запись очевидно подделана — её сохранить, в качестве доказательства попытки tampering.

## Связь с billing audit (ADR-0010)

Биллинг-events связаны с audit-events через `correlation_id`:

```sql
-- Для каждого billing_event должен быть audit_event с тем же ID
SELECT b.event_id, b.event_type, b.correlation_id,
       (a.id IS NOT NULL) AS audit_exists
FROM platform.billing_events b
LEFT JOIN tnt_<tenant>.audit_events a ON a.id = b.correlation_id
WHERE b.tenant_id = '<tenant>'
  AND b.recorded_at > now() - interval '30 days'
  AND a.id IS NULL;        -- расхождения

-- В обратную сторону — billable события без billing-записи
SELECT a.id, a.action, a.recorded_at
FROM tnt_<tenant>.audit_events a
LEFT JOIN platform.billing_events b ON b.correlation_id = a.id
WHERE a.action IN ('account.opened', 'ubo.verification.completed', 'premium.feature.activated')
  AND a.recorded_at BETWEEN '<from>' AND '<to>'
  AND b.event_id IS NULL;
```

`billing-service` запускает Temporal workflow `WeeklyReconciliation` (см. [ADR-0010 § Реконсиляция](../adr/0010-billing-and-audit-log.md)). Расхождения > 5 на тенанте за неделю → P1 алерт.

## 5-летнее хранение (115-ФЗ)

Audit log хранится **5 лет от даты события** (security-architecture § 8.1). Стратегия:

| Возраст | Где живёт | Доступ |
|---|---|---|
| 0–6 месяцев | hot tablespace, NVMe | <100ms запрос |
| 6–12 месяцев | warm tablespace, SSD | <1s |
| 12+ месяцев | cold storage (MinIO + Postgres FDW) | 1–10s |
| 5 лет + 1 день | архив доступен по запросу | через restore-procedure (TBD) |

Партицирование по `recorded_at` ежемесячно (DDL — см. `services/audit-service/migrations/`):

```sql
-- Пример partition swap для архивации
ALTER TABLE tnt_<tenant>.audit_events DETACH PARTITION audit_events_2025_03;
-- Перенести в cold storage:
COPY (SELECT * FROM audit_events_2025_03)
  TO PROGRAM 'mc pipe minio-cold/audit-archive/<tenant>/2025-03.csv.gz';
DROP TABLE audit_events_2025_03;
```

Backup: ежедневный full + continuous WAL (см. [`../operations/backup-restore.md`](../operations/backup-restore.md)).

## Регулярная проверка (еженедельная)

CronJob `audit-verifier-weekly`:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: audit-verifier-weekly
  namespace: platform
spec:
  schedule: "0 3 * * 0"               # воскресенье 03:00 UTC
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: verifier
              image: aibank/audit-verifier:1.0
              command: ["audit-verifier","verify-all","--alert-threshold=any-broken"]
```

Результат публикуется как metric `audit_chain_broken_count{tenant}` → Grafana + alert P0.

## Регуляторный запрос

При запросе ЦБ / ФинЦЕРТ / РКН «покажите audit за период X»:

```bash
# Экспорт + подпись отдельным regulator-tool (TBD)
audit-export \
  --tenant=<tenant> \
  --from=<ts> --to=<ts> \
  --filter-actions=decision.approved,decision.rejected \
  --output=/secure-snap/regulator-<id>.json \
  --sign-with-platform-key

# Output:
#   /secure-snap/regulator-<id>.json
#   /secure-snap/regulator-<id>.json.sig    -- подпись платформы
#   /secure-snap/regulator-<id>.proof.txt   -- proof of chain integrity
```

Передача регулятору — через DPO банка.

## Что НЕ делать

- НЕ модифицировать audit_events (даже если кажется, что есть «битая» запись)
- НЕ DROP TABLE audit_events для re-init (потеря 5-летнего след)
- НЕ передавать audit-export по незащищённым каналам (PII внутри)
- НЕ запускать ad-hoc DELETE в БД (триггер блокирует, но попытка = security event)

## Связанные документы

- [`security-architecture.md` § 8.2, § 8.3](../security-architecture.md) — что логируется и что нет
- [ADR-0010](../adr/0010-billing-and-audit-log.md) — audit ↔ billing связь
- [`incident-response.md`](incident-response.md) — chain-broken = P0 incident flow
- [`break-glass-procedure.md`](break-glass-procedure.md) — break-glass всегда оставляет audit-event
- [`../operations/backup-restore.md`](../operations/backup-restore.md) — backup стратегия audit

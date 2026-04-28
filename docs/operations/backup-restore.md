# Backup & Restore

## Когда использовать

- Настройка backup для нового кластера (SaaS / on-prem)
- Quarterly DR drill (обязательно per `security-architecture.md` § 11)
- Реальный disaster recovery
- Проверка backup-status перед апгрейдом
- Регуляторный запрос «как вы храните копии и восстанавливаетесь»

Ссылки: [`security-architecture.md` § 9.3](../security-architecture.md), [ADR-0010](../adr/0010-billing-and-audit-log.md), [ADR-0009](../adr/0009-on-prem-update-strategy.md).

## RPO / RTO

| Метрика | Цель | Источник |
|---|---|---|
| RPO (Recovery Point Objective — потеря данных) | **15 минут** | security-architecture § 9.3 |
| RTO (Recovery Time Objective — время восстановления) | **4 часа** | security-architecture § 9.3 |

Backup стратегия (общий принцип):

- **Ежедневно полные backup** всех stateful-компонентов
- **Continuous WAL** для PostgreSQL (incremental, RPO ≤ 15 мин)
- **Cold-site в другом регионе** (для SaaS) или второй DC (для on-prem банка)
- **3-2-1 правило:** 3 копии, 2 разных носителя, 1 offsite

## PostgreSQL

### Полный backup (ежедневно)

```bash
# Per-tenant pg_dump (для отдельного восстановления)
PGPASSWORD=$BACKUP_PWD pg_dump \
  -h <pg-host> -U backup -d aibank \
  --schema=tnt_<tenant> \
  --format=custom \
  --file=/backup/daily/<tenant>/$(date +%Y%m%d).dump

# Full cluster dump (платформа + все тенанты + temporal_visibility)
PGPASSWORD=$BACKUP_PWD pg_dumpall \
  -h <pg-host> -U backup \
  --file=/backup/daily/full-$(date +%Y%m%d).sql
gzip /backup/daily/full-*.sql

# Retention: 7 дней локально, 30 дней в cold storage, 5 лет audit-only
```

CronJob:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-daily-backup
  namespace: platform-data
spec:
  schedule: "30 1 * * *"          # 01:30 UTC, до maintenance window
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: postgres:16-alpine
              command: ["/scripts/backup-daily.sh"]
              env:
                - name: BACKUP_PWD
                  valueFrom: { secretKeyRef: { name: backup-creds, key: password } }
              volumeMounts:
                - { name: backups, mountPath: /backup }
          volumes:
            - name: backups
              persistentVolumeClaim: { claimName: backups-pvc }
```

### Continuous WAL archiving (RPO ≤ 15 мин)

```bash
# В postgresql.conf
archive_mode = on
archive_command = 'mc cp %p minio-cold/wal-archive/%f'
archive_timeout = 60                # форсировать каждые 60s
wal_keep_size = 16GB

# Verify archiving
PGPASSWORD=$DBA_PWD psql -h <pg> -U dba -c \
  "SELECT pg_walfile_name(pg_current_wal_lsn()), last_archived_wal, last_archived_time
   FROM pg_stat_archiver"
```

### Restore

#### Сценарий A: Полный restore в новый кластер

```bash
# 1. Создать новый кластер (Terraform)
cd infrastructure/terraform/environments/saas-prod-restore/
terraform apply

# 2. Restore base backup
PGPASSWORD=$DBA_PWD pg_restore \
  -h <new-pg> -U dba -d aibank \
  --clean --if-exists \
  /backup/daily/full-<latest>.dump

# 3. Apply WAL до точки T
mc cp -r minio-cold/wal-archive /tmp/wal-restore/
# В postgresql.conf на новом кластере:
#   restore_command = 'cp /tmp/wal-restore/%f %p'
#   recovery_target_time = '<T>'
# pg_ctl start

# 4. Verify
PGPASSWORD=$DBA_PWD psql -h <new-pg> -U dba -c \
  "SELECT count(*) FROM platform.tenants;
   SELECT max(recorded_at) FROM tnt_<tenant>.audit_events;"
```

#### Сценарий B: Restore одного тенанта

```bash
# 1. Создать пустую схему
PGPASSWORD=$DBA_PWD psql -h <pg> -U dba -d aibank -c \
  "CREATE SCHEMA tnt_<tenant>_restore"

# 2. Restore в неё
PGPASSWORD=$DBA_PWD pg_restore \
  -h <pg> -U dba -d aibank \
  --schema=tnt_<tenant> \
  --no-owner \
  /backup/daily/<tenant>/<yyyymmdd>.dump

# 3. Compare / promote
# ALTER SCHEMA tnt_<tenant> RENAME TO tnt_<tenant>_old_<ts>;
# ALTER SCHEMA tnt_<tenant>_restore RENAME TO tnt_<tenant>;

# 4. Audit-event обязательно
audit-cli emit --action platform.tenant_restored \
  --tenant=<tenant> --from-backup=<yyyymmdd>
```

#### Failover на standby (RTO < 30 мин)

```bash
# 1. Promote standby
PGPASSWORD=$DBA_PWD psql -h <standby> -U dba -c "SELECT pg_promote()"

# 2. Update DNS / connection strings
kubectl set env deploy --all -n platform \
  DATABASE_URL="postgres://...@<new-primary>:5432/aibank?sslmode=require"
kubectl rollout restart deploy --all -n platform

# 3. Verify
./scripts/smoke.sh

# 4. Старый primary — recreate as new standby
```

## MinIO / Object Storage

### Backup через mc mirror

```bash
# В крон, ежедневно
mc mirror --overwrite --remove \
  primary-minio/aibank-documents \
  backup-minio/aibank-documents-$(date +%Y%m%d)

# Cold storage retention 30 дней
```

### Backup через bucket replication (preferred for prod)

```bash
mc admin replicate add \
  primary-minio/aibank-documents \
  cold-minio/aibank-documents-replica

mc replicate ls primary-minio/aibank-documents
```

### Restore

```bash
mc mirror cold-minio/aibank-documents-<yyyymmdd> primary-minio/aibank-documents
```

## Kafka

### Retention + tiered storage

```bash
# В server.properties
log.retention.hours=168                   # 7 дней
log.retention.bytes=-1
remote.log.storage.system.enable=true     # KIP-405 tiered storage
remote.log.storage.manager.class.name=...
remote.storage.s3.bucket=kafka-tiered
remote.log.metadata.manager.class.name=...
```

### Snapshot topic config

```bash
# Backup topic configurations
kafka-configs.sh --bootstrap-server <kafka> --describe --entity-type topics \
  > /backup/kafka/topics-$(date +%Y%m%d).txt
kafka-acls.sh --bootstrap-server <kafka> --list \
  > /backup/kafka/acls-$(date +%Y%m%d).txt
```

### Restore

В классической Kafka — restore не делается; релайься на retention + tiered storage.
Если catastrophic loss — пересоздание topic'ов из конфига; consumer-offsets restore из metadata-snapshot.

## Vault

### Backup snapshot (encrypted)

```bash
# HA Vault — raft snapshot
kubectl exec -n platform-data vault-0 -- \
  vault operator raft snapshot save /tmp/vault-$(date +%Y%m%d).snap

# Скопировать в cold storage (snapshot уже encrypted Vault'ом)
kubectl cp platform-data/vault-0:/tmp/vault-$(date +%Y%m%d).snap \
  /backup/vault/vault-$(date +%Y%m%d).snap

mc cp /backup/vault/vault-*.snap cold-minio/vault-snapshots/
```

### Restore

```bash
# 1. Скачать snapshot
mc cp cold-minio/vault-snapshots/vault-<yyyymmdd>.snap /tmp/

# 2. Restore (в running unsealed cluster)
kubectl cp /tmp/vault-<yyyymmdd>.snap platform-data/vault-0:/tmp/restore.snap
kubectl exec -n platform-data vault-0 -- \
  vault operator raft snapshot restore /tmp/restore.snap

# 3. Verify
kubectl exec -n platform-data vault-0 -- vault status
kubectl exec -n platform-data vault-0 -- vault kv list platform/
```

## Temporal

Temporal-state — это PostgreSQL схема `temporal_visibility`. Включена в общий PG backup.
Отдельных команд **не нужно**.

При restore PostgreSQL Temporal автоматически перечитает event history.
Подробности в [`../runbooks/temporal-recovery.md`](../runbooks/temporal-recovery.md).

## Qdrant

```bash
# Snapshot collection
curl -X POST "http://qdrant:6333/collections/tnt_<tenant>_rag/snapshots"

# List snapshots
curl "http://qdrant:6333/collections/tnt_<tenant>_rag/snapshots"

# Download
curl "http://qdrant:6333/collections/tnt_<tenant>_rag/snapshots/<snapshot_name>" \
  -o /backup/qdrant/<tenant>-$(date +%Y%m%d).snap
```

## Audit log — особый случай (5 лет)

Audit log стрит обычный PG-backup, но **retention 5 лет** (115-ФЗ). См. также [`../runbooks/audit-log-integrity.md`](../runbooks/audit-log-integrity.md).

| Возраст | Где живёт | Backup частота |
|---|---|---|
| 0–6 мес | hot tablespace | ежедневный full + WAL |
| 6–12 мес | warm tablespace | weekly full + monthly cold copy |
| 12–60 мес | cold storage (MinIO / S3 Glacier-equiv) | monthly verification |
| 60+ мес | архив для уничтожения с актом | по запросу |

## Quarterly DR drill checklist

Каждый квартал — обязательный drill (per `security-architecture.md` § 11). Полный сценарий:

- [ ] **T-30 дней**: scheduled, банк-клиент уведомлён (если on-prem; SaaS — внутренне)
- [ ] **T-7 дней**: подтвердить наличие свежих backups (full + WAL ≤ 15 мин lag)
- [ ] **T+0**:
  - [ ] Симулировать loss primary PG (`docker stop` или drop standby promotion)
  - [ ] Failover на standby — измерить RTO
  - [ ] Сравнить: данные в standby vs ожидаемое — измерить RPO
- [ ] Восстановить один тенант из backup в isolated environment
  - [ ] Сравнить count(*) ключевых таблиц с исходным
  - [ ] Audit-chain integrity (audit-verifier)
- [ ] Vault snapshot restore в test-cluster
- [ ] MinIO restore одного bucket
- [ ] Прогнать smoke.sh в restored environment
- [ ] Запостить отчёт: actual RPO / RTO vs SLO; gaps; action items
- [ ] Обновить runbook'и, если что-то пошло не так

## Backup verification (еженедельно)

```bash
# Verify последний PG-backup открывается
PGPASSWORD=$DBA_PWD pg_restore --list /backup/daily/full-<latest>.dump | head -20

# Verify Vault snapshot не corrupt
sha256sum /backup/vault/vault-<latest>.snap
# Сравнить с записанным sha256

# MinIO replication status
mc admin replicate status primary-minio/aibank-documents

# Archived WAL — есть ли gaps
PGPASSWORD=$DBA_PWD psql -h <pg> -U dba -c \
  "SELECT * FROM pg_stat_archiver"
```

CronJob `backup-verifier-weekly` пишет результат в Prometheus metric `backup_verification_status`.

## Что НЕ покрывает этот runbook

- **Application-level state migration** — это часть upgrade-runbook'а
- **Cross-region failover SaaS** — отдельный runbook (TBD, требует DR-design второго региона)
- **Restore конкретной audit-записи** — она immutable, restore = forensic-snapshot
- **Restore Temporal workflow state** при corruption — см. `../runbooks/temporal-recovery.md` § «Recovery после сбоя cluster»

## Связанные документы

- [`security-architecture.md` § 9.3, § 11](../security-architecture.md) — DR-требования
- [`../runbooks/incident-response.md`](../runbooks/incident-response.md) — общий поток инцидентов
- [`../runbooks/temporal-recovery.md`](../runbooks/temporal-recovery.md) — для Temporal-recovery
- [`../runbooks/audit-log-integrity.md`](../runbooks/audit-log-integrity.md) — audit retention
- [`monitoring-alerts.md`](monitoring-alerts.md) — backup-метрики и алерты
- [ADR-0009](../adr/0009-on-prem-update-strategy.md) — backup в blue-green-флоу

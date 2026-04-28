# Monitoring & Alerts

## Когда использовать

- Настройка наблюдаемости в новом окружении
- Проверка / расширение SLO для нового сервиса
- Дебаг алерта (что он значит, как погасить, что делать)
- Подготовка дашбордов для нового тенанта
- Аудит банка спрашивает «как вы понимаете, что всё работает»

Ссылки: [`technical-structure.md` § 3.5](../technical-structure.md), [`security-architecture.md` § 8.4](../security-architecture.md).

## Стек (LGTM, всё self-hosted)

| Компонент | Назначение | Эндпоинт (default) |
|---|---|---|
| **VictoriaMetrics** | Метрики (Prometheus-совместимые) | `vm.platform.svc:8428` |
| **Loki** | Логи | `loki.platform.svc:3100` |
| **Tempo** | Distributed tracing (OpenTelemetry) | `tempo.platform.svc:3200` |
| **Grafana** | UI / dashboards | `grafana.platform.svc:3000` |
| **Alertmanager** | Маршрутизация алертов | `alertmanager.platform.svc:9093` |
| **OpenTelemetry collector** | Aggregation tracing/logs | `otel-collector.platform.svc:4317` |

## Ключевые SLI

| SLI | Метрика | Где смотреть |
|---|---|---|
| **Доступность сервиса** | `up{job="<svc>"}`, `probe_success` | Grafana → «Service health» |
| **p95 / p99 latency per endpoint** | `histogram_quantile(0.95, http_request_duration_seconds_bucket)` | Grafana → «Latency» |
| **Error rate per tenant** | `sum(rate(http_requests_total{status=~"5..", tenant_id="<t>"}[5m])) / sum(rate(http_requests_total{tenant_id="<t>"}[5m]))` | Grafana → «Per-tenant errors» |
| **Audit log ingest lag** | `time() - audit_last_recorded_at_seconds` | Grafana → «Audit health» |
| **LLM token consumption per tenant per model** | `sum(rate(llm_tokens_total[1h])) by (tenant_id, model)` | Grafana → «LLM Usage» |
| **Temporal workflow latency** | `temporal_workflow_endtoend_latency_seconds_bucket` | Grafana → «Temporal» |
| **PostgreSQL replication lag** | `pg_replication_lag_seconds` | Grafana → «Postgres» |
| **Kafka consumer lag** | `kafka_consumergroup_lag` | Grafana → «Kafka» |
| **GPU utilization** | `nvidia_smi_utilization_gpu_ratio` | Grafana → «AI / GPU» |
| **Audit chain integrity** | `audit_chain_broken_count` | Grafana → «Audit health» |

## SLO targets (placeholder, согласовать с CSM и банком per-tenant)

| SLO | Цель | Категория |
|---|---|---|
| Доступность платформы (any tenant) | **99.9%** monthly | P0 |
| Доступность критичных сервисов (api-gateway, identity, audit) | **99.95%** monthly | P0 |
| p95 latency `/v1/applications` (создание заявки) | **<1s** | P1 |
| p95 latency полного onboarding-флоу | **<30s** для ИП, **<5min** для ООО | P1 |
| Error rate per tenant | **<0.5%** | P1 |
| Audit log ingest lag | **<60s** | P0 |
| RPO | **15 мин** | DR |
| RTO | **4 часа** | DR |
| LLM gateway p95 latency | **<3s** для chat, **<10s** для vision | P1 |
| Temporal workflow lag (pending tasks) | **<100** items | P1 |

Per-tenant SLA — отдельный override в `configs/tenants/<tenant>/sla.yaml`.

## Alert categories (per security-architecture § 8.4)

| Категория | SLA реакции | Канал |
|---|---|---|
| **P0 (15 мин)** | Compromise, утечка, audit chain broken, full outage | PagerDuty + Telegram `#sec-oncall` + Mattermost call |
| **P1 (1 час)** | Operational issue: cert expired, Vault down, error rate breach | Telegram `#oncall` + email |
| **P2 (1 день)** | Hygiene: outdated deps, security findings, low-priority capacity | Email weekly digest |

## Alert rules (Prometheus / VictoriaMetrics format)

### P0 — Critical

```yaml
groups:
  - name: aibank-p0
    interval: 30s
    rules:
      - alert: ServiceDown
        expr: up{job=~"tenant-service|audit-service|api-gateway|identity-service|onboarding-orchestrator"} == 0
        for: 2m
        labels: { severity: P0 }
        annotations:
          summary: "{{ $labels.job }} is down"
          runbook: "docs/runbooks/incident-response.md#инцидент-2"

      - alert: AuditChainBroken
        expr: audit_chain_broken_count > 0
        for: 0m
        labels: { severity: P0 }
        annotations:
          summary: "Audit chain broken for tenant {{ $labels.tenant_id }}"
          runbook: "docs/runbooks/audit-log-integrity.md"

      - alert: AuditIngestLagHigh
        expr: time() - audit_last_recorded_at_seconds > 60
        for: 2m
        labels: { severity: P0 }
        annotations:
          summary: "Audit ingest lag > 60s — events may be lost"
          runbook: "docs/runbooks/audit-log-integrity.md"

      - alert: PostgresReplicationLag
        expr: pg_replication_lag_seconds > 30
        for: 5m
        labels: { severity: P0 }
        annotations:
          summary: "PG replication lag > 30s — failover risk"
          runbook: "docs/runbooks/incident-response.md#инцидент-4"

      - alert: PIIInLogs
        expr: |
          sum(rate({service="llm-gateway"} |~ "(?i)\\b\\d{12}\\b" [5m])) > 0
          or sum(rate({service="llm-gateway"} |~ "(?i)passport.*\\d{4}.*\\d{6}" [5m])) > 0
        labels: { severity: P0 }
        annotations:
          summary: "Possible PII in llm-gateway logs"
          runbook: "docs/runbooks/llm-gateway-troubleshooting.md#сценарий-3"
```

### P1 — High

```yaml
  - name: aibank-p1
    interval: 1m
    rules:
      - alert: ErrorRateHigh
        expr: |
          (sum by (tenant_id) (rate(http_requests_total{status=~"5.."}[5m]))
           /
           sum by (tenant_id) (rate(http_requests_total[5m]))) > 0.005
        for: 10m
        labels: { severity: P1 }
        annotations:
          summary: "Error rate > 0.5% for tenant {{ $labels.tenant_id }}"

      - alert: LatencyP95High
        expr: |
          histogram_quantile(0.95, sum by (le, endpoint) (
            rate(http_request_duration_seconds_bucket[5m])
          )) > 1
        for: 10m
        labels: { severity: P1 }
        annotations:
          summary: "p95 latency > 1s on {{ $labels.endpoint }}"

      - alert: TemporalWorkflowStuck
        expr: |
          time() - temporal_workflow_running_started_seconds > 3600
          and on() temporal_workflow_status == 1
        for: 5m
        labels: { severity: P1 }
        annotations:
          summary: "Workflow {{ $labels.workflow_id }} running > 1h"
          runbook: "docs/runbooks/temporal-recovery.md#проблема-1"

      - alert: LLMGatewayErrorRate
        expr: |
          sum(rate(llm_request_errors_total[5m]))
          / sum(rate(llm_requests_total[5m])) > 0.05
        for: 5m
        labels: { severity: P1 }
        annotations:
          summary: "LLM gateway error rate > 5%"
          runbook: "docs/runbooks/llm-gateway-troubleshooting.md"

      - alert: VaultDown
        expr: vault_up == 0
        for: 2m
        labels: { severity: P1 }
        annotations:
          summary: "Vault unreachable"

      - alert: CertExpiresSoon
        expr: probe_ssl_earliest_cert_expiry - time() < 14*24*3600
        labels: { severity: P1 }
        annotations:
          summary: "TLS cert expires in <14 days for {{ $labels.instance }}"

      - alert: DBMigrationFailed
        expr: db_migrator_failed_count > 0
        for: 0m
        labels: { severity: P1 }
        annotations:
          summary: "{{ $value }} tenants failed migration"
          runbook: "docs/runbooks/db-migration.md#что-делать-при-partial-failure"
```

### P2 — Medium

```yaml
  - name: aibank-p2
    interval: 5m
    rules:
      - alert: BackupNotRecent
        expr: time() - backup_last_success_seconds > 24*3600
        for: 30m
        labels: { severity: P2 }
        annotations:
          summary: "Backup not successful in last 24h"

      - alert: DependencyVulnHigh
        expr: dependency_vulnerabilities_high > 0
        for: 1h
        labels: { severity: P2 }
        annotations:
          summary: "{{ $value }} HIGH severity vulnerabilities found"

      - alert: KafkaLagGrowing
        expr: kafka_consumergroup_lag > 10000
        for: 30m
        labels: { severity: P2 }

      - alert: BillingReconciliationDrift
        expr: billing_reconciliation_drift_count > 5
        for: 1h
        labels: { severity: P2 }
        annotations:
          summary: "{{ $value }} billing/audit drift events"
```

## Channel routing (Alertmanager)

```yaml
route:
  receiver: default
  group_by: [alertname, tenant_id]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - match: { severity: P0 }
      receiver: pagerduty-p0
      continue: true
    - match: { severity: P0 }
      receiver: telegram-sec-oncall
    - match: { severity: P1 }
      receiver: telegram-oncall
    - match: { severity: P2 }
      receiver: email-digest

receivers:
  - name: pagerduty-p0
    pagerduty_configs:
      - service_key: <TBD-paging-key>
  - name: telegram-sec-oncall
    webhook_configs:
      - url: https://api.telegram.org/bot<TBD>/sendMessage?chat_id=<TBD-sec-oncall>
  - name: telegram-oncall
    webhook_configs:
      - url: https://api.telegram.org/bot<TBD>/sendMessage?chat_id=<TBD-oncall>
  - name: email-digest
    email_configs:
      - to: oncall-digest@aibank.ru
```

Для on-prem банков канал может быть Mattermost вместо Telegram (per `configs/tenants/<bank>/notifications.yaml`).

## Dashboards (минимальный набор)

| Dashboard | Цель | Виджеты |
|---|---|---|
| **Platform Overview** | Главный, на стене ops | Up/down per service, RPS, error rate, p95 |
| **Service Health** | Per-service deep dive | Latency, errors, saturation, dependencies |
| **Per-Tenant** | Per-tenant view | Apps created, decisions, p95, errors |
| **Audit Health** | Audit pipeline | Ingest rate, lag, chain integrity |
| **LLM Usage** | AI-слой | Tokens, cost, model distribution per tenant |
| **Temporal** | Workflow orchestration | Running workflows, queue lag, activity errors |
| **PostgreSQL** | DB | Replication lag, slow queries, connection pool |
| **Kafka** | Event bus | Throughput, consumer lag, partition health |
| **GPU / AI Infra** | vLLM pool | GPU util, KV-cache usage, batch size |
| **Backup / DR** | Backup-status | Last success, size, restore-test result |
| **Security** | SIEM-light | Failed auth, suspicious outbound, anomalies |
| **Billing** | Финансовые события | Per-tenant counts, reconciliation drift |

JSON-файлы дашбордов — в `infrastructure/grafana/dashboards/` (TBD).

## Логи (Loki labels)

Каждая строка лога имеет минимум:

| Label | Значение |
|---|---|
| `service` | `tenant-service`, `audit-service`, ... |
| `tenant_id` | `bank-alpha`, `demo`, ... (если применимо) |
| `level` | `info` / `warn` / `error` |
| `trace_id` | OTel trace для correlation |
| `pod` | `<deploy>-<hash>` |
| `namespace` | `platform` / `platform-ai` / ... |

LogQL-примеры:

```logql
# Все ошибки последний час
{level="error"} |= "" | json | __error__ = ""

# Только tenant=bank-alpha
{tenant_id="bank-alpha"} |= "5xx"

# Pattern: PII like ИНН (используется в alert)
{service="llm-gateway"} |~ "(?i)\\b\\d{12}\\b"
```

Retention: application logs 1 год (security-architecture § 8.1).

## Tracing

OpenTelemetry SDK во всех сервисах. Trace propagation — стандарт W3C.

```bash
# В Grafana → Explore → Tempo → Trace ID
# Из логов: trace_id="abc123..."

# Service map: Grafana → Service Graph (из Tempo)
```

Sampling: 10% production, 100% staging, 100% при `x-debug: 1` header.

## Связанные документы

- [`technical-structure.md` § 3.5](../technical-structure.md) — observability stack rationale
- [`security-architecture.md` § 8](../security-architecture.md) — что логируется, что нет
- [`../runbooks/incident-response.md`](../runbooks/incident-response.md) — что делать с алертом
- [`backup-restore.md`](backup-restore.md) — backup-метрики и алерты
- [`onboarding-engineer.md`](onboarding-engineer.md) — где смотреть дашборды первый день

# SaaS Deployment

## Когда использовать

- Развёртывание платформы в Yandex Cloud / VK Cloud (новая среда)
- Создание нового тенанта в существующем SaaS-кластере
- Releаз апгрейд (weekly cadence per `technical-structure.md` § 11)
- Восстановление после disaster (DR-drill или real DR)

Ссылки: [`technical-structure.md` § 10.1](../technical-structure.md), [`infrastructure/helm/README.md`](../../infrastructure/helm/README.md).

## Целевая инфраструктура

```
[Yandex Cloud / VK Cloud — РФ-регион]
├── Managed K8s 1.30+
│   ├── platform namespace        — общие сервисы
│   ├── tenant-<id> namespaces    — изоляция по тенанту
│   ├── platform-ai namespace     — GPU pool (vLLM)
│   └── platform-data namespace   — Temporal + Vault (если managed недостаточно)
├── Managed PostgreSQL 16          — отдельная БД на тенанта (multi-tenant внутри одной БД per ADR-0002)
├── Managed Kafka                  — KRaft mode
├── Managed S3 / MinIO в кластере
├── Managed Redis 7
└── DNS:
    ├── api.aibank.ru              — внешний edge
    ├── <tenant>.platform.aibank.ru — per-tenant subdomain routing
    └── admin-<tenant>.aibank.ru   — банковская админка
```

## Pre-requisites

- Terraform-state в `infrastructure/terraform/environments/saas-<env>/`
- KUBECONFIG к целевому кластеру (через Yandex Cloud CLI / VK Cloud CLI)
- Helm 3.14+
- Доступ к container registry (`cr.yandex.cloud/<our-folder>` или эквивалент)
- Подписи cosign-ключом (для production-overlay)

## Первичный деплой (новая среда)

### Шаг 1. Provision инфраструктуры

```bash
cd infrastructure/terraform/environments/saas-prod/
terraform init
terraform plan -out=tfplan
# Review плана — особенно sensitive ресурсы
terraform apply tfplan

# Output:
#   k8s_cluster_endpoint, kubeconfig_path
#   postgres_endpoint, postgres_password (в Vault)
#   kafka_bootstrap_servers
#   minio_endpoint
```

### Шаг 2. Bootstrap секретов в Vault

```bash
# Vault deploy (если не managed)
helm install vault hashicorp/vault \
  -n platform-data --create-namespace \
  -f infrastructure/helm/charts/vault/values-saas.yaml

# Initial unseal (3-of-5 keys)
kubectl exec -n platform-data vault-0 -- vault operator init -key-shares=5 -key-threshold=3
# Сохранить keys в физический safe!

# Загрузить platform-secrets
vault kv put platform/postgres/dba password=<...>
vault kv put platform/registry/cosign-pub-key value=@cosign.pub
```

### Шаг 3. Применить platform-миграции

```bash
# Через Job (предпочтительно)
kubectl create job db-migrator-bootstrap \
  --image=cr.yandex.cloud/<folder>/aibank/db-migrator:1.5.0 \
  -n platform -- platform --service tenant-service \
  --source /migrations/tenant-service \
  --dsn "$DATABASE_URL"

kubectl wait --for=condition=complete --timeout=10m \
  job/db-migrator-bootstrap -n platform
```

Подробности — [`../runbooks/db-migration.md`](../runbooks/db-migration.md).

### Шаг 4. Установка платформы через Helm

```bash
cd infrastructure/helm

# Resolve dependencies
helm dependency update platform-saas/

# Lint и render для review
helm lint platform-saas/
helm template aibank platform-saas/ -f platform-saas/values.yaml \
  -f overlays/saas-prod/values.yaml > /tmp/manifest.yaml
# Review /tmp/manifest.yaml

# Установка
helm install aibank platform-saas/ \
  --namespace aibank-saas --create-namespace \
  -f platform-saas/values.yaml \
  -f overlays/saas-prod/values.yaml \
  --wait --timeout=20m
```

### Шаг 5. Smoke verification

```bash
# Из bastion-pod, доступного в кластер
cd /opt/aibank
./scripts/smoke.sh
# Использует endpoints port-forward'ом или service internal DNS
```

Подробности — `scripts/smoke.sh` README в репо. 9 шагов; в случае сбоя последний шаг записан в `/tmp/aibank-smoke-state/last-step`.

## Создание нового тенанта

```bash
# 1. Подготовить tenant config (см. configs/tenants/_template/)
cp -r configs/tenants/_template configs/tenants/bank-alpha
$EDITOR configs/tenants/bank-alpha/tenant.yaml
$EDITOR configs/tenants/bank-alpha/integrations/abs.yaml

# 2. Загрузить tenant-secrets в Vault
vault kv put platform/tenants/bank-alpha/abs username=... password=...
vault kv put platform/tenants/bank-alpha/esia client_id=... client_secret=...

# 3. Создать тенант через CLI
tenant-cli create \
  --id=bank-alpha \
  --name="Альфа-Банк" \
  --bik=044525593 \
  --inn=7728168971 \
  --deployment-mode=saas

# 4. Применить tenant-миграции для всех сервисов в релизе
for svc in tenant-service audit-service onboarding-orchestrator document-service \
           ubo-service client-service risk-engine billing-service; do
  kubectl create job db-migrator-tenant-bank-alpha-$svc \
    --image=cr.yandex.cloud/<folder>/aibank/db-migrator:1.5.0 \
    -n platform -- tenant \
      --tenant-id=bank-alpha --service=$svc \
      --source=/migrations/$svc/tenant
  kubectl wait --for=condition=complete --timeout=5m \
    job/db-migrator-tenant-bank-alpha-$svc -n platform
done

# 5. Per-tenant Helm release (если используем релиз-на-тенант)
helm install aibank-bank-alpha platform-saas/ \
  --namespace tenant-bank-alpha --create-namespace \
  -f platform-saas/values.yaml \
  -f configs/tenants/bank-alpha/values.yaml

# 6. DNS-record (через Terraform или ручной)
# bank-alpha.platform.aibank.ru → ingress IP

# 7. Smoke against tenant
./scripts/smoke.sh
# (модифицированный, с TENANT_ID=bank-alpha)
```

## Health checks per service

| Сервис | URL | Ожидаемый ответ |
|---|---|---|
| tenant-service | `/health` | 200 + `{"status":"ok","version":"..."}` |
| audit-service | `/health` | 200 + последняя hash-position |
| identity-service | `/health` | 200, OIDC discovery в `/.well-known/openid-configuration` |
| document-service | `/health` | 200, MinIO connectivity |
| onboarding-orchestrator | `/health` | 200, Temporal cluster reachable |
| risk-engine | `/health` | 200, rules.yaml loaded |
| client-service / ubo-service | `/health` | 200 |
| api-gateway | `/health`, `/ready` | 200 |
| bff-onboarding / bff-admin | `/health`, `/graphql` | 200, schema loaded |
| llm-gateway | `/healthz`, `/readiness` | 200, backends reachable |
| agent-* | `/healthz` | 200, llm-gateway reachable |
| ext-egrul / ext-rosfinmon / ext-fssp | `/health` | 200, redis connectivity |
| abs-connector / abs-adapter-* | `/health` | 200 |
| billing-service | `/health` | 200, Kafka consumer лагом < 60s |

```bash
# Quick health-check всех сервисов
for svc in tenant audit identity document onboarding risk client ubo api-gateway billing; do
  echo -n "$svc: "
  kubectl exec -n aibank-saas deploy/$svc-service -- \
    wget -qO- http://localhost:8080/health || echo "FAIL"
done
```

## Шаблон production values.yaml

`infrastructure/helm/overlays/saas-prod/values.yaml`:

```yaml
global:
  imageTag: "1.5.0"
  imagePullPolicy: IfNotPresent
  imagePullSecrets:
    - name: cr-yandex-cloud
  imageRegistry: cr.yandex.cloud/<folder>/aibank
  environment: prod

# Каждый сервис как alias aibank-service
tenant-service:
  replicaCount: 3
  resources:
    requests: { cpu: 200m, memory: 256Mi }
    limits:   { cpu: 1000m, memory: 1Gi }
  hpa:
    enabled: true
    minReplicas: 3
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
  podDisruptionBudget:
    enabled: true
    minAvailable: 2
  networkPolicy:
    enabled: true
  serviceMonitor:
    enabled: true
  config:
    DATABASE_URL: "postgres://...@<managed-pg>:5432/aibank?sslmode=require"
    VAULT_ADDR: "http://vault.platform-data:8200"
  existingSecret: tenant-service-secrets

audit-service:
  replicaCount: 3
  # ...

llm-gateway:
  replicaCount: 2
  resources:
    requests: { cpu: 1, memory: 2Gi }
    limits:   { cpu: 4, memory: 8Gi }
  config:
    LLM_GATEWAY_FORCE_MOCK: "0"
  existingSecret: llm-gateway-secrets

# Stateful — managed (no chart dependency in production)
postgres:
  enabled: false  # используем managed
redis:
  enabled: false
kafka:
  enabled: false
minio:
  enabled: false
temporal:
  enabled: true   # self-hosted, не managed в Yandex Cloud
  replicaCount: 3
  persistence:
    storageClass: yc-network-ssd
    size: 100Gi

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: api.aibank.ru
      paths: [{ path: /, pathType: Prefix }]
  tls:
    - secretName: api-aibank-tls
      hosts: [api.aibank.ru]
```

## Релизный апгрейд (weekly)

```bash
# 1. CI prepares release tag, pushes images
# 2. CI запускает db-migrator (ADR-0005)
# 3. Helm upgrade (rolling)

helm upgrade aibank platform-saas/ \
  --namespace aibank-saas \
  -f platform-saas/values.yaml \
  -f overlays/saas-prod/values.yaml \
  --set global.imageTag=1.6.0 \
  --wait --timeout=20m \
  --atomic     # автоматический rollback при неудаче

# 4. Smoke
./scripts/smoke.sh

# 5. Если smoke fail — `--atomic` уже rolled back, дополнительно
helm rollback aibank --namespace aibank-saas
```

## Recovery / DR (см. `../operations/backup-restore.md`)

RPO 15 мин / RTO 4 часа. Cold-site в другом регионе. Quarterly drill — обязательно.

## Связанные документы

- [`on-prem-deploy.md`](on-prem-deploy.md) — on-prem (другой flow)
- [`airgapped-bundle.md`](airgapped-bundle.md) — для on-prem distribution
- [`../runbooks/db-migration.md`](../runbooks/db-migration.md) — миграции
- [`../runbooks/incident-response.md`](../runbooks/incident-response.md) — что делать при upgrade fail
- [`../operations/monitoring-alerts.md`](../operations/monitoring-alerts.md) — после деплоя — что мониторить
- [`infrastructure/helm/README.md`](../../infrastructure/helm/README.md) — chart structure

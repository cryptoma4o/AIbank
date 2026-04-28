# On-Prem Deployment

## Когда использовать

- Первичная установка платформы в инфраструктуре банка-клиента
- Квартальный апгрейд (per ADR-0009)
- Подготовка к pilot-deploy у нового банка
- Disaster recovery в банковском контуре

Ссылки: [ADR-0009 — стратегия обновлений](../adr/0009-on-prem-update-strategy.md), [`technical-structure.md` § 10.2](../technical-structure.md), [`airgapped-bundle.md`](airgapped-bundle.md).

## Pre-requisites (банк готовит)

| Компонент | Минимум | Рекомендация |
|---|---|---|
| Kubernetes | 1.28+ (vanilla, OKD, Deckhouse) | 1.30+, Deckhouse-CE |
| Istio | 1.20+ (для blue-green VS) | 1.22+ |
| PostgreSQL | 16.x | 16, HA-кластер 3 ноды |
| Redis | 7.x | 7, sentinel или cluster |
| Kafka | 3.5+ (KRaft) | 3.7+, 3 broker |
| MinIO | RELEASE.2024-01+ | latest stable |
| Vault | 1.15+ | HA-mode 3+ ноды |
| GPU | Опционально (если AI on-prem) | A100 / H100 / Ascend 910B |
| Storage | 500GB SSD | NVMe + replicated |
| K8s ingress | Nginx / banking ingress | Nginx + cert-manager |
| Network | white-list outbound (для ext-API) | banking proxy с whitelist ЕГРЮЛ/ЕСИА/Росфинмон |

## Топология (per ADR-0009)

```
[Кластер банка-клиента]
├── platform-blue/           — текущая активная версия
├── platform-green/          — параллельная новая (во время апгрейда)
├── platform-data/           — stateful (PG, Kafka, Vault, Temporal, Qdrant)
├── platform-mesh/           — Istio gateway + VirtualService
└── platform-ai/             — vLLM, GPU pool, LLM weights
```

## Первичная установка

### Шаг 1. Получение и верификация bundle

```bash
# Bundle получен по согласованному каналу (физический носитель / banking proxy)
# Шаги верификации — см. airgapped-bundle.md § Verification

cosign verify-blob \
  --key /opt/aibank/keys/aibank-release.pub \
  --signature aibank-platform-1.5.0-onprem.tar.gz.sig \
  aibank-platform-1.5.0-onprem.tar.gz

tar -xzf aibank-platform-1.5.0-onprem.tar.gz
cd aibank-platform-1.5.0-onprem

# Проверка digest каждого артефакта
yq -r '.images[] | .name + " " + .digest' manifest.yaml | while read name digest; do
  actual=$(sha256sum "docker-images/${name}.tar" | awk '{print $1}')
  [[ "$actual" == "$digest" ]] || { echo "FAIL: $name"; exit 1; }
done
```

### Шаг 2. Загрузка images в banking registry

```bash
for img in docker-images/**/*.tar; do
  docker load -i "$img"
  IMAGE=$(docker load -i "$img" | grep -oE 'aibank/[^ ]+' | head -1)
  docker tag $IMAGE bank-registry.local/$IMAGE
  docker push bank-registry.local/$IMAGE
done
```

### Шаг 3. SBOM scan (banking-ИБ)

```bash
# Banking ИБ запускает свой scanner (Trivy / Snowflake / банковский)
for img in docker-images/**/*.tar; do
  trivy image --severity HIGH,CRITICAL --format json \
    --input "$img" > "scan-results/$(basename $img .tar).json"
done

# Результаты передаются ИБ-комитету банка для approval
```

### Шаг 4. Подготовка namespaces

```bash
kubectl create namespace platform-mesh
kubectl create namespace platform-data
kubectl create namespace platform-ai
kubectl create namespace platform-blue       # для blue-green

kubectl label namespace platform-blue istio-injection=enabled
kubectl label namespace platform-green istio-injection=enabled
kubectl label namespace platform-mesh istio-injection=enabled
```

### Шаг 5. Stateful services в `platform-data`

Если банк предоставляет managed-сервисы — пропустить, использовать banking endpoints в values.yaml.
Если банк хочет в кластере — установка из bundle:

```bash
# PostgreSQL (если не банковский)
helm install postgres bundle/helm-charts/postgres \
  -n platform-data \
  -f bundle/values-onprem-postgres.yaml

# Vault HA
helm install vault bundle/helm-charts/vault \
  -n platform-data \
  -f bundle/values-onprem-vault.yaml

# Vault unseal — 3-of-5 банковских ключей
kubectl exec -n platform-data vault-0 -- vault operator init -key-shares=5 -key-threshold=3
# Ключи передаются банковскому SOC, неудаляемо

# Kafka, Temporal, MinIO, Qdrant аналогично
```

### Шаг 6. Tenant config

```bash
# Из bundle или подготовленный банком configs/tenants/<bank>/
mkdir -p /opt/aibank/configs/tenants/<bank>
cp -r bundle/configs/_template/* /opt/aibank/configs/tenants/<bank>/

# Заполнить (банком + нашим CSM):
$EDITOR /opt/aibank/configs/tenants/<bank>/tenant.yaml
$EDITOR /opt/aibank/configs/tenants/<bank>/integrations/abs.yaml
$EDITOR /opt/aibank/configs/tenants/<bank>/risk-policy/rules.yaml
$EDITOR /opt/aibank/configs/tenants/<bank>/sla.yaml
```

Структура tenant config — см. `configs/tenants/_template/` и [`tenant-configuration.md`](../tenant-configuration.md).

### Шаг 7. Migrations

```bash
kubectl apply -f bundle/manifests/db-migrator-job.yaml -n platform-data
kubectl wait --for=condition=complete --timeout=30m \
  job/db-migrator-1.5.0 -n platform-data

kubectl logs -n platform-data job/db-migrator-1.5.0 | tail -50
# Должно быть "Applied N migrations to platform schema"
# и "Applied K migrations to tenant schemas (X tenants OK)"
```

### Шаг 8. Helm install platform

```bash
helm install aibank bundle/helm-charts/platform-onprem \
  --namespace platform-blue \
  -f bundle/values-onprem-defaults.yaml \
  -f /opt/aibank/configs/tenants/<bank>/values.yaml \
  --set global.imageTag=1.5.0 \
  --set global.imageRegistry=bank-registry.local/aibank \
  --wait --timeout=30m
```

### Шаг 9. Istio VirtualService

```bash
# Создать VS с активным blue
kubectl apply -f bundle/manifests/istio-vs-blue.yaml -n platform-mesh

# Verify
kubectl get virtualservice -n platform-mesh
istioctl analyze -n platform-mesh
```

### Шаг 10. Smoke verification

```bash
# Из bastion-pod банка
cd /opt/aibank
./scripts/smoke.sh
# Все 9 шагов должны пройти зелёным
```

## Квартальный апгрейд (blue-green)

Полный flow — [ADR-0009 § «Часть 2. Flow обновления»](../adr/0009-on-prem-update-strategy.md). Краткий runbook ниже.

### T-7 дней — банк проверяет bundle

Banking DevOps:
- Проверяет подпись (cosign)
- Распаковывает в pre-prod
- Прогоняет smoke
- Подтверждает «готовы к окну»

### T+0 (maintenance window 02:00–04:00 МСК)

#### Шаг 1. Migrations

```bash
kubectl apply -f bundle-1.6.0/manifests/db-migrator-job.yaml -n platform-data
kubectl wait --for=condition=complete --timeout=30m job/db-migrator-1.6.0 -n platform-data
```

Миграции **forward-compatible** (ADR-0005) — blue работает на новой схеме.

#### Шаг 2. Deploy green

```bash
helm install aibank bundle-1.6.0/helm-charts/platform-onprem \
  --namespace platform-green --create-namespace \
  -f bundle-1.6.0/values-onprem-defaults.yaml \
  -f /opt/aibank/configs/tenants/<bank>/values.yaml \
  --set global.imageTag=1.6.0 \
  --wait --timeout=20m
```

#### Шаг 3. Smoke против green (закрытый VS)

```bash
# Smoke-VS направляет /v1/__smoke на green
kubectl apply -f bundle-1.6.0/manifests/istio-vs-smoke-green.yaml -n platform-mesh

# Прогон smoke с x-release-test header
RELEASE_TEST=true ./scripts/smoke.sh

# Если fail — investigate, не активировать
```

#### Шаг 4. Активация green

```bash
tenant-cli release activate \
  --target=green \
  --version=1.6.0 \
  --tenant=<bank> \
  --canary=none

# Output:
#   switching VS blue → green ... done
#   audit_event_id: <uuid>
#   active: green (1.6.0) since <ts>
```

Под капотом — `kubectl apply` нового `platform-vs-green.yaml` с weights 0/100 для всех hosts.

#### Шаг 5. Мониторинг 30 минут

```bash
tenant-cli release status --tenant=<bank>
# Output:
#   active: green (1.6.0)
#   error_rate_5m: 0.3%
#   p95_latency: 142ms
#   audit_chain: unbroken
```

При проблеме — rollback в 5–10 минут:

```bash
tenant-cli release rollback --tenant=<bank>
# audit_event platform.release_rolled_back записан
```

#### Шаг 6. T+24 часа — scale down blue

```bash
helm uninstall aibank --namespace platform-blue
# Но namespace не удалять ещё 7 дней
```

#### Шаг 7. T+7 дней — finalize

```bash
kubectl delete namespace platform-blue
# В следующем релизе текущий green становится blue
```

## Connection без Istio

Если банк не использует Istio — fallback через nginx ingress. См. ADR-0009 § «Истанбульская специфика»; runbook `upgrade-no-istio.md` (TBD). Slower rollback (1–2 минуты), feature parity не 100%.

## Backup verification (post-deploy)

```bash
# 1. Postgres backup есть
PGPASSWORD=$BACKUP_PWD psql -h <pg> -U dba -d aibank -c \
  "SELECT pg_size_pretty(pg_database_size('aibank')), now() - max(applied_at)
   FROM platform.backup_log"

# 2. MinIO replication (если включена)
mc admin info bank-minio

# 3. Vault snapshot
kubectl exec -n platform-data vault-0 -- vault operator raft snapshot save /tmp/snap-test.snap
ls -la <bank-storage>/vault-snapshots/ | tail -3

# 4. Temporal backup (это часть PG backup, отдельно не нужен)
```

Подробности — [`../operations/backup-restore.md`](../operations/backup-restore.md).

## Контрольный список банка перед go-live

- [ ] K8s 1.28+ установлен и доступен
- [ ] Istio установлен (или принято решение `no-istio` режим)
- [ ] PostgreSQL HA (3 ноды) с TDE
- [ ] Vault HA (3 ноды) unsealed
- [ ] Banking registry имеет все images, прошедшие SBOM scan
- [ ] DNS-record созданы (`api.<bank-domain>`, `admin.<bank-domain>`)
- [ ] cert-manager + Let's Encrypt / банковский CA настроен
- [ ] Outbound whitelist для ext-API: `egrul.nalog.ru`, `esia.gosuslugi.ru`, `<rosfinmon-endpoint>`, `<fssp>`, `<bank-abs>`
- [ ] On-call контакты согласованы (наша + банковская)
- [ ] DR-drill scheduled на Q+1
- [ ] Smoke прошёл

## Связанные документы

- [ADR-0009](../adr/0009-on-prem-update-strategy.md) — стратегия blue-green
- [ADR-0005](../adr/0005-db-migrations-strategy.md) — forward-compatible миграции
- [`airgapped-bundle.md`](airgapped-bundle.md) — формирование и верификация bundle
- [`saas-deploy.md`](saas-deploy.md) — для сравнения с SaaS-флоу
- [`../runbooks/incident-response.md`](../runbooks/incident-response.md) — если апгрейд упал
- [`../operations/backup-restore.md`](../operations/backup-restore.md) — backup стратегия
- [`tenant-configuration.md`](../tenant-configuration.md) — структура per-tenant config

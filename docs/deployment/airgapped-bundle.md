# Air-Gapped Bundle

## Когда использовать

- Подготовка квартального bundle для on-prem-банка (наша сторона)
- Hot-fix bundle между квартальными релизами
- Верификация полученного bundle (банковская сторона)
- Подготовка пилотного bundle для нового design-partner

Ссылки: [ADR-0009 § «Часть 5. Формат bundle»](../adr/0009-on-prem-update-strategy.md), [`technical-structure.md` § 10.2](../technical-structure.md).

## Состав bundle

```
aibank-platform-<version>-onprem.tar.gz
├── manifest.yaml                  — список артефактов с digest и versions
├── manifest.yaml.sig              — cosign-подпись
├── docker-images/
│   ├── platform/
│   │   ├── tenant-service-<v>.tar
│   │   ├── audit-service-<v>.tar
│   │   ├── identity-service-<v>.tar
│   │   ├── document-service-<v>.tar
│   │   ├── onboarding-orchestrator-<v>.tar
│   │   ├── risk-engine-<v>.tar
│   │   ├── client-service-<v>.tar
│   │   ├── ubo-service-<v>.tar
│   │   ├── api-gateway-<v>.tar
│   │   ├── bff-onboarding-<v>.tar
│   │   ├── bff-admin-<v>.tar
│   │   ├── billing-service-<v>.tar
│   │   ├── notification-service-<v>.tar
│   │   ├── ext-egrul-<v>.tar
│   │   ├── ext-rosfinmon-<v>.tar
│   │   ├── ext-fssp-<v>.tar
│   │   └── abs-connector-<v>.tar
│   ├── adapters/
│   │   ├── abs-adapter-cft-<v>.tar       — может быть своя версия (ADR-0006)
│   │   ├── abs-adapter-diasoft-<v>.tar
│   │   └── abs-adapter-rs-bank-<v>.tar
│   └── ai/
│       ├── llm-gateway-<v>.tar
│       ├── agent-document-intake-<v>.tar
│       ├── agent-reconciliation-<v>.tar
│       ├── agent-ubo-tracing-<v>.tar
│       ├── agent-conversational-<v>.tar
│       └── rag-service-<v>.tar
├── helm-charts/
│   ├── platform-onprem-<v>.tgz             — umbrella chart
│   └── charts/
│       ├── aibank-service-0.1.0.tgz
│       ├── llm-gateway-0.1.0.tgz
│       ├── postgres-0.1.0.tgz              — values-only wrapper
│       ├── redis-0.1.0.tgz
│       ├── kafka-0.1.0.tgz
│       ├── minio-0.1.0.tgz
│       ├── temporal-0.1.0.tgz
│       └── vault-0.1.0.tgz
├── migrations/
│   └── db-migrator-<v>.tar                  — Job-образ с embedded миграциями
├── models/                                   — только если LLM веса изменились
│   ├── gemma-4-26b-vision-<model_v>/
│   │   ├── model.safetensors
│   │   ├── tokenizer.json
│   │   └── config.json
│   ├── qwen-3.5-7b-<model_v>/
│   ├── bge-m3-embed-<model_v>/
│   └── checksum.txt
├── manifests/
│   ├── db-migrator-job.yaml
│   ├── istio-vs-blue.yaml
│   ├── istio-vs-green.yaml
│   ├── istio-vs-smoke-green.yaml
│   └── networkpolicy-blue-green.yaml
├── tenant-cli/
│   ├── tenant-cli-linux-amd64
│   ├── tenant-cli-linux-amd64.sig
│   └── tenant-cli-checksum.txt
├── runbooks/
│   ├── upgrade.md                            — пошаговая для банк-DevOps
│   ├── rollback.md
│   ├── verify.md
│   ├── upgrade-no-istio.md
│   └── recovery.md
├── values/
│   ├── values-onprem-defaults.yaml
│   ├── values-onprem-postgres.yaml
│   ├── values-onprem-vault.yaml
│   └── _template/
│       └── values-tenant-<bank>.yaml
├── configs/
│   └── _template/                            — tenant config template
│       ├── tenant.yaml
│       ├── integrations/
│       ├── risk-policy/
│       └── ai/
├── sbom/
│   ├── platform/
│   │   ├── tenant-service-<v>.spdx.json
│   │   └── ...
│   └── ai/
│       └── ...
└── changelog.md
```

## Storage requirements

| Часть | Объём (приблизительно) |
|---|---|
| Docker images (платформа + адаптеры + AI) | 8–12 GB |
| Helm charts | <50 MB |
| LLM weights — full set (Gemma-4-26B + Qwen-3.5-7B + bge-m3) | 30–60 GB |
| Migrations | <10 MB |
| SBOM | <100 MB |
| Manifest, runbooks, configs | <10 MB |
| **Итого с весами** | **40–75 GB** |
| **Итого без весов (если не изменились)** | **8–13 GB** |

Bundle поставляется как одиночный signed `tar.gz` (не split).

## Procedure: build bundle (наша сторона)

### Шаг 1. Trigger через CI

```bash
# Запуск из main после merge release-tag
gitlab-ci pipeline run --branch=release/1.6.0 --variable BUILD_ONPREM_BUNDLE=true

# Или ручной запуск (для hotfix)
nx run-many -t build:onprem-bundle --version=1.6.1 --hotfix=true
```

### Шаг 2. Сборка артефактов

```bash
# 1. Все docker images
nx run-many -t docker-build --tag=1.6.0
nx run-many -t docker-save --output=./bundle/docker-images

# 2. Helm charts
helm package infrastructure/helm/platform-onprem \
  -d ./bundle/helm-charts \
  --version=1.6.0
helm dependency update infrastructure/helm/platform-onprem
for chart in infrastructure/helm/charts/*; do
  helm package "$chart" -d ./bundle/helm-charts/charts
done

# 3. Migrations
docker save aibank/db-migrator:1.6.0 -o ./bundle/migrations/db-migrator-1.6.0.tar

# 4. LLM weights — только если изменились
if changed_since=$(git diff --name-only v1.5.0..v1.6.0 -- ai/llm-gateway/configs/models.yaml); then
  ./tools/bundle-models.sh --output=./bundle/models
fi

# 5. SBOM
for img in $(find bundle/docker-images -name "*.tar"); do
  syft scan docker-archive:$img -o spdx-json > ./bundle/sbom/$(basename $img .tar).spdx.json
done

# 6. tenant-cli binary
GOOS=linux GOARCH=amd64 go build -o ./bundle/tenant-cli/tenant-cli-linux-amd64 ./tools/tenant-cli
sha256sum ./bundle/tenant-cli/tenant-cli-linux-amd64 > ./bundle/tenant-cli/tenant-cli-checksum.txt
```

### Шаг 3. Manifest

```yaml
# manifest.yaml
version: 1.6.0
build_id: gl-pipeline-87623
built_at: 2026-04-26T15:32:18Z
git_commit: abc123def...
images:
  - name: tenant-service
    tag: 1.6.0
    digest: sha256:fedcba...
    file: docker-images/platform/tenant-service-1.6.0.tar
  - name: llm-gateway
    tag: 1.6.0
    digest: sha256:1234...
    file: docker-images/ai/llm-gateway-1.6.0.tar
  # ... все остальные
helm_charts:
  - name: platform-onprem
    version: 1.6.0
    digest: sha256:...
    file: helm-charts/platform-onprem-1.6.0.tgz
models:
  - name: gemma-4-26b-vision
    version: 1.3
    digest: sha256:abcd...
    size_gb: 28
    changed_since_previous: true
  - name: qwen-3.5-7b
    version: 1.0
    digest: sha256:efgh...
    size_gb: 14
    changed_since_previous: false
sbom:
  format: spdx-json
  files: sbom/**/*.spdx.json
runbooks:
  - upgrade.md
  - rollback.md
  - verify.md
compatibility:
  k8s_min: "1.28.0"
  istio_min: "1.20.0"
  postgres_min: "16.0"
  previous_bundle_version: "1.5.0"
release_notes: changelog.md
```

### Шаг 4. Подпись

```bash
# Cosign main signature
cosign sign-blob \
  --key file:.aibank/release-key \
  manifest.yaml > manifest.yaml.sig

# Каждый docker-image (image content sha256 уже в manifest)
# Не нужно подписывать каждый .tar отдельно — manifest.yaml.sig покрывает

# Опциональная ГОСТ-подпись для банков, требующих этого (см. ADR-0009)
gostsign --key=.aibank/gost-2012.key \
  --in=manifest.yaml --out=manifest.yaml.gost.sig
```

### Шаг 5. Упаковка

```bash
cd bundle
tar -czf ../aibank-platform-1.6.0-onprem.tar.gz .
sha256sum ../aibank-platform-1.6.0-onprem.tar.gz > ../aibank-platform-1.6.0-onprem.tar.gz.sha256
```

### Шаг 6. Доставка

| Канал | Когда применять |
|---|---|
| Физический носитель (USB-HSM-encrypted) | Default для new pilot |
| Banking secure proxy с whitelist | Если банк имеет outbound на наш CDN |
| Banking SFTP с GPG | Альтернатива для compatible банков |

Notify банк-DevOps по согласованному каналу с reference ID.

## Procedure: verify bundle (банковская сторона)

### Шаг 1. Проверка подписи

```bash
cosign verify-blob \
  --key /opt/aibank/keys/aibank-release.pub \
  --signature aibank-platform-1.6.0-onprem.tar.gz.sig \
  aibank-platform-1.6.0-onprem.tar.gz

# Если ГОСТ-подпись (опционально)
gostverify --key=/opt/aibank/keys/aibank-release-gost.pub \
  --in=manifest.yaml --sig=manifest.yaml.gost.sig
```

При несовпадении — НЕ распаковывать. Связаться с нашей релизной командой.

### Шаг 2. SHA-256 архива

```bash
sha256sum aibank-platform-1.6.0-onprem.tar.gz
# Сравнить с aibank-platform-1.6.0-onprem.tar.gz.sha256 из канала доставки
```

### Шаг 3. Распаковка

```bash
mkdir aibank-platform-1.6.0-onprem
tar -xzf aibank-platform-1.6.0-onprem.tar.gz -C aibank-platform-1.6.0-onprem/
cd aibank-platform-1.6.0-onprem
```

### Шаг 4. Проверка digest каждого артефакта

```bash
# Скрипт включён в bundle (./verify.sh)
./verify.sh

# Альтернативно — вручную
yq -r '.images[] | .file + " " + .digest' manifest.yaml | while read file expected; do
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "sha256:$expected" ]] || { echo "FAIL: $file"; exit 1; }
done
yq -r '.helm_charts[] | .file + " " + .digest' manifest.yaml | while read file expected; do
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "sha256:$expected" ]] || { echo "FAIL: $file"; exit 1; }
done
yq -r '.models[]? | .file + " " + .digest' manifest.yaml | while read file expected; do
  [[ -f "$file" ]] || continue        # некоторые могут отсутствовать (changed_since_previous=false)
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "sha256:$expected" ]] || { echo "FAIL: $file"; exit 1; }
done
```

### Шаг 5. SBOM scan (banking ИБ)

```bash
# Trivy or banking equivalent
trivy sbom \
  --severity HIGH,CRITICAL \
  --format json \
  sbom/platform/tenant-service-1.6.0.spdx.json > scan-results/tenant-service.json

# Aggregate report
trivy sbom --severity HIGH,CRITICAL sbom/**/*.spdx.json > scan-results/aggregate.txt
```

ИБ комитет банка делает решение «approve / reject». При reject — наша команда выпускает hotfix bundle с исправлением.

### Шаг 6. Pre-prod test

Развернуть bundle в pre-prod кластере банка, прогнать smoke. Только после успеха — окно production.

```bash
# В pre-prod
./bundle/runbooks/upgrade.md  # или скрипт
# ...
./scripts/smoke.sh
```

### Шаг 7. Подтверждение готовности

Banking DevOps шлёт нам:
- Acknowledged digest manifest (тот же хэш, что мы подписали)
- Pre-prod smoke result
- Дата окна production

После этого квартальное окно может быть открыто.

## Hot-fix bundle

Упрощённая версия — только изменённые images, без LLM weights и без миграций (если не нужны):

```bash
# CI
nx run-many -t build:onprem-bundle --hotfix=true \
  --base-version=1.6.0 --hotfix-version=1.6.1 \
  --include-services=tenant-service,risk-engine
```

Размер hotfix bundle: <500 MB. Цикл подготовки сжат до 4 часов.

## Storage у банка (рекомендация)

Хранить **4 последних bundle** для возможности отката на 2 релиза назад. Это ответственность банка; мы документируем рекомендацию.

```
/opt/aibank/bundles/
├── 1.4.0/  (oldest, can be deleted)
├── 1.5.0/
├── 1.6.0/  (current)
└── 1.6.1-hotfix/
```

## Связанные документы

- [ADR-0009 § «Часть 5»](../adr/0009-on-prem-update-strategy.md) — формальное определение формата
- [`on-prem-deploy.md`](on-prem-deploy.md) — что делать с верифицированным bundle
- [`saas-deploy.md`](saas-deploy.md) — для сравнения с SaaS-флоу (не использует bundle)
- [`../runbooks/db-migration.md`](../runbooks/db-migration.md) — миграции внутри bundle
- [`technical-structure.md` § 10.2](../technical-structure.md) — air-gapped принципы
- [cosign documentation](https://docs.sigstore.dev/cosign/overview/)

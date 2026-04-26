<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# infrastructure/helm/

## Purpose

Helm-чарты для развёртывания всех сервисов платформы. Два верхнеуровневых чарта-обёртки (`platform-saas`, `platform-onprem`) с разными дефолтами для режимов развёртывания. Сами чарты сервисов — в `charts/`. Статус: **директория пустая, чарты создаются с Фазы 1–2**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор Helm-чартов: структура umbrella charts, правила именования, политика секретов |

## Planned Structure

```
helm/
├── platform-saas/
│   ├── Chart.yaml          # umbrella chart для SaaS
│   └── values.yaml         # SaaS-defaults (managed DB, cloud S3)
├── platform-onprem/
│   ├── Chart.yaml          # umbrella chart для on-prem
│   └── values.yaml         # on-prem-defaults (self-hosted Kafka, MinIO)
└── charts/                 # чарты отдельных сервисов
    ├── api-gateway/
    ├── tenant-service/
    ├── identity-service/
    ├── onboarding-orchestrator/
    ├── risk-engine/
    ├── document-service/
    ├── llm-gateway/
    ├── temporal/            # Temporal Server
    └── ...
```

## For AI Agents

### Working In This Directory

- Secrets в values.yaml **запрещены** — только ссылки на Vault (`vault://kv/...`) или External Secrets Operator.
- Каждый чарт должен иметь `NetworkPolicy` и `PodDisruptionBudget`.
- Resource requests/limits обязательны для каждого контейнера — без них Pod не пройдёт admission webhook.
- Образы — pinned by digest (`image: registry/name@sha256:...`), не latest.
- Для on-prem: все зависимости (PostgreSQL, Kafka, Redis) должны быть доступны как sub-chart или внешний endpoint.
- Tenancy isolation: у каждого тенанта свой K8s namespace (`tenant-{id}`). Чарт должен поддерживать параметр `tenantNamespace`.

### Testing Requirements

- `helm lint charts/{name}` на каждый PR
- `helm template charts/{name} | kubeval --kubernetes-version 1.30.0`
- `helm upgrade --dry-run` на staging перед prod

### Common Patterns

Обязательные labels для каждого чарта:
```yaml
labels:
  app.kubernetes.io/name: {{ .Chart.Name }}
  app.kubernetes.io/version: {{ .Chart.AppVersion }}
  app.kubernetes.io/managed-by: ArgoCD
  platform/tenant-id: {{ .Values.tenantId | default "platform" }}
```

## Dependencies

### Internal
- `../terraform/` — Terraform создаёт K8s кластер, в который деплоятся чарты
- `../../configs/` — ArgoCD читает конфиги тенантов и применяет через helm values

### External
- Helm 3.x, ArgoCD, External Secrets Operator, cert-manager

<!-- MANUAL: -->

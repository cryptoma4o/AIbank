<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# infrastructure/

## Purpose

Infrastructure as Code (IaC) для развёртывания платформы. Поддерживает два режима: **SaaS** (Yandex Cloud / VK Cloud) и **on-prem** (контур банка, включая air-gapped). Один и тот же артефакт разворачивается в обоих режимах — это архитектурный принцип.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор инфраструктурного слоя: режимы развёртывания (SaaS/on-prem), используемые инструменты |

## Subdirectories

| Директория | Назначение |
|-----------|-----------|
| `helm/` | Helm-чарты всех сервисов платформы. Отдельные values для SaaS и on-prem (см. `helm/AGENTS.md`) |
| `terraform/` | Облачная инфраструктура — Yandex Cloud / VK Cloud: K8s кластер, Managed PostgreSQL, Kafka, S3, сети (см. `terraform/AGENTS.md`) |
| `ansible/` | Bare-metal автоматизация для on-prem развёртываний: установка Kubernetes (Deckhouse / OKD), настройка GPU-нод, сетевой конфигурации (см. `ansible/AGENTS.md`) |

## For AI Agents

### Working In This Directory

**Критические ограничения:**

- Запрещены **managed-сервисы AWS/GCP** в коде — только то, что работает в любой среде (PostgreSQL, Kafka, Redis, MinIO). Это архитектурный принцип гибридного развёртывания.
- Kubernetes — **везде**: и SaaS, и on-prem. Версия 1.30+.
- Service mesh — **Istio**: mTLS между сервисами обязателен (требование аудита). Не заменять на Linkerd или другие без ADR.
- Secrets — только через **HashiCorp Vault**. Никогда в values.yaml или Terraform state.
- Все изменения конфигов — через **GitOps (ArgoCD)**. Никакого прямого `kubectl apply` в production.
- On-prem российские дистрибутивы K8s: **Deckhouse** или **OKD** (Red Hat OpenShift) — предпочтительны для банков с требованием реестра Минцифры.

**Топология SaaS:**
```
Yandex Cloud
├── K8s cluster
│   ├── platform namespace (общие сервисы)
│   ├── tenant-{id} namespace (изоляция тенантов)
│   └── ai namespace (GPU-pool: A100/H100)
├── Managed PostgreSQL (отдельная БД на тенанта)
├── Managed Kafka
└── Managed S3 или MinIO в кластере
```

**Топология on-prem:**
```
Контур банка
├── K8s cluster (Deckhouse / OKD)
│   ├── platform namespace
│   ├── ai namespace (GPU-ноды банка: H20 / Ascend 910B)
│   └── adapters namespace
├── PostgreSQL банка
├── Kafka в кластере
├── MinIO в кластере
└── Vault или интеграция с банковским HSM
```

**Air-gapped bundle** для on-prem (формируется при релизе):
- Tarball docker-образов всех сервисов
- Helm-чарты
- Веса LLM-моделей (safetensors)
- Скрипт установки

### Testing Requirements

- Terraform: `terraform validate` + `tflint` в CI
- Helm: `helm lint` + `helm template` + `kubeval` для проверки манифестов
- Ansible: `ansible-lint` + molecule тесты для ролей
- Smoke tests после deploy на staging: проверка health endpoints всех сервисов

### Common Patterns

- Именование Kubernetes ресурсов: `{сервис}-{env}` (например, `tenant-service-prod`)
- Labels обязательны: `app.kubernetes.io/name`, `app.kubernetes.io/version`, `tenant-id` (где применимо)
- Resource limits обязательны для каждого Pod'а (предотвращение noisy neighbor между тенантами)
- NetworkPolicy для каждого namespace (deny-all default, allow-list явных соединений)

## Dependencies

### Internal
- `../services/` — Docker-образы сервисов
- `../ai/` — Docker-образы AI-сервисов (GPU-ноды)
- `../configs/` — конфигурации тенантов (применяются через ArgoCD)

### External
- Kubernetes 1.30+, Istio, cert-manager, Nginx Ingress
- HashiCorp Vault (HA), Keycloak
- VictoriaMetrics, Grafana, Loki, Tempo (LGTM-стек)
- Yandex Cloud / VK Cloud (SaaS)
- Deckhouse / OKD (on-prem)

<!-- MANUAL: -->

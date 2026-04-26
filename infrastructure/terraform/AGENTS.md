<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# infrastructure/terraform/

## Purpose

Terraform-конфигурации для облачной инфраструктуры SaaS-развёртывания. Основной провайдер — **Yandex Cloud** (с поддержкой VK Cloud как альтернативы). Управляет: K8s-кластером, managed PostgreSQL, Kafka, S3, сетями, IAM. Статус: **директория пустая, конфигурации создаются с Фазы 1**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор Terraform-конфигураций: провайдеры, структура модулей, remote state, именование ресурсов |

## Planned Structure

```
terraform/
├── modules/
│   ├── k8s-cluster/        # YC Managed Kubernetes
│   ├── postgres/           # YC Managed PostgreSQL (1 БД на тенанта)
│   ├── kafka/              # YC Managed Kafka
│   ├── s3/                 # YC Object Storage (1 bucket на тенанта)
│   ├── vpc/                # Сети, подсети, security groups
│   └── vault/              # HashiCorp Vault cluster
├── environments/
│   ├── staging/
│   └── production/
└── tenant-provisioning/    # Terraform для создания нового тенанта
```

## For AI Agents

### Working In This Directory

- Terraform state — **только в remote backend** (Yandex Object Storage с шифрованием и блокировкой). Никогда локально.
- Secrets в `.tf` файлах **запрещены** — только через Vault provider или переменные среды CI.
- Каждый тенант — **изолированный набор ресурсов**: отдельная PostgreSQL БД, отдельный S3 bucket с IAM-политиками, отдельный K8s namespace.
- Использовать **Terraform modules** для переиспользуемых компонентов; не копировать блоки ресурсов.
- `terraform plan` обязателен в CI перед `apply`; `apply` — только через pipeline с manual approval для production.
- Теги обязательны для всех ресурсов: `environment`, `tenant-id` (если тенантный ресурс), `managed-by=terraform`.

### Testing Requirements

- `terraform validate` + `tflint` на каждый PR
- `terraform plan` с сохранением плана как артефакта CI
- Checkov или tfsec для security проверок (обязательно перед merge)
- `terraform apply` на staging автоматически, на production — с ручным аппрувом

### Common Patterns

Именование ресурсов: `{env}-{компонент}-{тенант-id}` (например, `prod-postgres-alfa-bank`)

Структура модуля тенанта:
```hcl
module "tenant" {
  source    = "../../modules/tenant-provisioning"
  tenant_id = "alfa-bank"
  env       = "production"
  # Каждый вызов создаёт изолированный набор ресурсов
}
```

## Dependencies

### Internal
- Создаёт инфраструктуру, которую используют `../helm/` чарты
- Выходные переменные (endpoints, credentials refs) передаются в Helm values через CI pipeline

### External
- Terraform 1.6+, Yandex Cloud provider, VK Cloud provider (альтернатива)
- Checkov / tfsec для security scanning

<!-- MANUAL: -->

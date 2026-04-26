# infrastructure/

Infrastructure-as-Code: Helm-чарты, Terraform-модули, Ansible-роли.

См. [docs/technical-structure.md](../docs/technical-structure.md), раздел 10.

## Структура

- `helm/` — Helm-чарты для развёртывания платформы
  - `platform-saas/` — конфигурация для SaaS-режима
  - `platform-onprem/` — конфигурация для on-prem-развёртывания
  - `charts/` — чарты отдельных сервисов
- `terraform/` — облачная инфраструктура (Yandex Cloud, VK Cloud)
- `ansible/` — bare-metal развёртывание для on-prem банков

## Принципы

- Один артефакт деплоя для SaaS и on-prem (разные `values.yaml`)
- Никаких managed-сервисов в коде — только portable компоненты
- Air-gapped bundle для on-prem с предзагруженными образами и весами моделей

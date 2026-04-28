# Deployment Documentation

Документация по развёртыванию платформы AIbank в SaaS- и on-prem-режимах.

## Когда использовать

- Первичная установка платформы для нового тенанта (SaaS) или у нового банка-клиента (on-prem)
- Релизный апгрейд (квартальный для on-prem, weekly для SaaS)
- Подготовка air-gapped bundle для on-prem
- Onboarding нового DevOps-инженера в проект

## Каталог

| Документ | Когда использовать |
|---|---|
| [`saas-deploy.md`](saas-deploy.md) | Установка/апгрейд платформы в Yandex Cloud / VK Cloud |
| [`on-prem-deploy.md`](on-prem-deploy.md) | Установка/апгрейд в инфраструктуре банка-клиента (per ADR-0009) |
| [`airgapped-bundle.md`](airgapped-bundle.md) | Сборка signed bundle для on-prem распространения |

## Архитектура развёртывания

| Аспект | SaaS | On-prem |
|---|---|---|
| Cloud / hardware | Yandex Cloud / VK Cloud | Банк предоставляет K8s 1.28+ |
| Релизный цикл | Continuous deploy на staging, weekly на prod | Quarterly bundle |
| Доставка артефактов | Container registry (РФ) + Helm chart repo | Air-gapped tarball, signed |
| Update strategy | Rolling + canary через Argo Rollouts | Blue-green per ADR-0009 |
| Secrets | Vault HA в кластере | Vault или банковский HSM |
| Monitoring | LGTM stack (наш) | LGTM stack (банковский, или поставляем мы) |

## Ссылки

- [`technical-structure.md` § 10](../technical-structure.md) — топология SaaS / on-prem
- [`technical-structure.md` § 11](../technical-structure.md) — CI/CD
- [ADR-0009](../adr/0009-on-prem-update-strategy.md) — стратегия обновлений on-prem
- [`infrastructure/helm/README.md`](../../infrastructure/helm/README.md) — Helm chart layout
- [`scripts/smoke.sh`](../../scripts/smoke.sh) — e2e smoke test

## Owner

DevOps-лид (релизы) + Tech-lead backend (сервис-конфиги). PR — review обоих.

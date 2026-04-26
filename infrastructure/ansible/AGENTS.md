<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-04-26 | Updated: 2026-04-26 -->

# infrastructure/ansible/

## Purpose

Ansible-плейбуки и роли для автоматизации on-prem развёртываний на bare-metal серверах банка. Используется когда банк предоставляет собственные серверы без облачного оркестратора. Статус: **директория пустая, плейбуки создаются с Фазы 7 (первый пилот)**.

## Key Files

| Файл | Описание |
|------|----------|
| `README.md` | Обзор Ansible-автоматизации: целевые ОС, перечень плейбуков и ролей, air-gapped режим |

## Planned Structure

```
ansible/
├── inventories/
│   ├── staging/
│   └── production/
├── roles/
│   ├── k8s-node/           # Подготовка ноды: ОС, container runtime, kubeadm
│   ├── gpu-node/           # Настройка GPU-нод (NVIDIA/Ascend драйверы, CUDA)
│   ├── storage/            # MinIO, локальный persistent storage
│   ├── network/            # Сетевая конфигурация, firewall rules
│   ├── vault/              # Установка HashiCorp Vault или интеграция с банковским HSM
│   └── cryptopro/          # Установка КриптоПро CSP (для УКЭП)
├── playbooks/
│   ├── site.yml            # Полная установка
│   ├── k8s-cluster.yml     # Только K8s
│   └── gpu-setup.yml       # Только GPU-ноды
└── air-gapped/             # Скрипты для установки без доступа в интернет
```

## For AI Agents

### Working In This Directory

- Целевые ОС: **Astra Linux** и **РЕД ОС** — российские дистрибутивы, обязательные для банков-субъектов КИИ (187-ФЗ).
- Kubernetes для on-prem: **Deckhouse** (отечественный) или **OKD** — предпочтительнее vanilla kubeadm для соответствия реестру Минцифры.
- GPU-ноды: поддержка **NVIDIA H20** и **Ascend 910B** (Huawei) — часто используются в российских банках.
- Air-gapped режим: все зависимости (rpm/deb пакеты, docker образы, pip wheels) должны быть в локальном репозитории. Плейбуки не должны обращаться в интернет при установке.
- Secrets (пароли к БД, сертификаты) — **только через Ansible Vault**, не в plaintext в inventory.
- КриптоПро CSP: лицензия — у банка-клиента, мы только автоматизируем установку через официальный установщик.

### Testing Requirements

- `ansible-lint` на каждый PR
- Molecule тесты для каждой роли (Docker или Vagrant)
- Smoke test после установки: проверка health всех сервисов, доступность Vault, работоспособность УКЭП

### Common Patterns

Идемпотентность: все задачи должны быть идемпотентны (повторный запуск не ломает систему).

Структура роли:
```
roles/k8s-node/
├── tasks/main.yml
├── handlers/main.yml
├── defaults/main.yml    # переменные с дефолтами
├── vars/main.yml        # неизменяемые переменные
├── templates/           # Jinja2 шаблоны конфигов
└── molecule/            # тесты роли
```

## Dependencies

### Internal
- Создаёт bare-metal инфраструктуру, на которую деплоятся `../helm/` чарты

### External
- Ansible 2.15+, ansible-lint, molecule
- Deckhouse / OKD (K8s дистрибутивы)
- Astra Linux / РЕД ОС (целевые ОС)
- КриптоПро CSP 5.x

<!-- MANUAL: -->

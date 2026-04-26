# Onboarding Platform

White-label платформа цифрового онбординга юридических лиц (ИП, ООО, АО) для российских банков.

## Документация

Вся проектная документация — в [`docs/`](docs/README.md). Начните оттуда.

| Если вы... | Идите сюда |
|---|---|
| Только что присоединились к проекту | [docs/README.md](docs/README.md) — порядок чтения |
| Хотите понять, что мы строим и для кого | [docs/product-vision.md](docs/product-vision.md) |
| Архитектор / тех-лид | [docs/technical-structure.md](docs/technical-structure.md) |
| ML-инженер | [docs/ai-agents-automation.md](docs/ai-agents-automation.md) |
| Backend-инженер | [docs/domain-model.md](docs/domain-model.md) + [docs/tenant-configuration.md](docs/tenant-configuration.md) |
| Security-инженер | [docs/security-architecture.md](docs/security-architecture.md) + [docs/compliance-map.md](docs/compliance-map.md) |
| Юрист / комплаенс | [docs/compliance-map.md](docs/compliance-map.md) |
| Хотите понять, как принимаются решения | [docs/adr/](docs/adr/README.md) |

## Структура репозитория

```
.
├── README.md                # этот файл
├── CHANGELOG.md             # значимые изменения
├── docs/                    # вся документация
│   └── adr/                 # architecture decision records
├── apps/                    # фронтенд-приложения
├── services/                # бэкенд-сервисы
├── ai/                      # ИИ-сервисы и агенты
├── packages/                # shared-библиотеки
├── infrastructure/          # IaC: Helm, Terraform, Ansible
├── configs/                 # конфигурации тенантов
└── tools/                   # внутренние тулы и моки
```

Подробное описание структуры — в [docs/technical-structure.md](docs/technical-structure.md), раздел 2.

## Быстрый старт для разработчика

> Раздел будет дополняться по мере появления первых сервисов.

```bash
# 1. Клонировать репо
git clone <url>
cd onboarding-platform

# 2. Установить зависимости (по мере появления стека)
# TBD

# 3. Запустить локальную среду
# TBD

# 4. Прочитать документацию
open docs/README.md
```

## Status

🚧 **Pre-MVP.** Идёт первая фаза — формализация домена, контрактов и базовой инфраструктуры.

См. [docs/technical-structure.md](docs/technical-structure.md), раздел 12 (План реализации по фазам).

## Лицензия

TBD. Решение о лицензировании платформы и её модулей принимается до первого релиза.

## Контакты

TBD. Указать ответственных по доменам после формирования команды.

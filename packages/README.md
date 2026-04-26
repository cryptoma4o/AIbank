# packages/

Shared-библиотеки, используемые несколькими сервисами.

См. [docs/technical-structure.md](../docs/technical-structure.md), раздел 2.

## Планируемые пакеты

- `domain-model/` — каноническая доменная модель (TS + Go + Python)
- `proto/` — gRPC-протоколы
- `openapi/` — OpenAPI-схемы REST
- `ui-kit/` — React-компоненты (whitelabel-ready)
- `tenant-config-schema/` — JSON Schema конфигов тенанта
- `audit-sdk/` — SDK для записи в audit log
- `rule-engine/` — движок декларативных правил
- `crypto-utils/` — криптография (ГОСТ + UEFI)

## Принципы

- Все пакеты типизированы
- Каждое breaking change — major version + ADR
- Доменная модель генерируется из единого источника схем

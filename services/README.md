# services/

Бэкенд-сервисы (Go в основном, частично Java для legacy АБС-адаптеров).

См. [docs/technical-structure.md](../docs/technical-structure.md), раздел 4.

## Категории сервисов

- **Edge:** `api-gateway/`, `bff-onboarding/`, `bff-admin/`
- **Core:** `tenant-service/`, `identity-service/`, `onboarding-orchestrator/`, `risk-engine/`, `document-service/`, `ubo-service/`, `client-service/`
- **Integration:** `abs-connector/`, `abs-adapter-{cft,diasoft,rs-bank}/`, `ext-{egrul,esia,rosfinmon,spark}/`
- **Support:** `notification-service/`, `audit-service/`, `billing-service/`

Каждый сервис — отдельная папка со своим Dockerfile, OpenAPI/proto-схемой и README.

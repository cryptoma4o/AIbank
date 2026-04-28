# AIbank Demo для банков

Готовый комплект для презентации платформы потенциальному банку-партнёру:
3 бизнес-сценария онбординга, скрипты для запуска end-to-end, и
структура slide-deck.

**Цель**: показать CISO / CTO / Compliance officer'у банка:

1. **Что платформа делает** (бизнес-flow от заявки до открытия счёта)
2. **Где AI-агенты работают** (document intake, UBO tracing, risk scoring, conversational)
3. **Как устроена security** (audit log, hash chain, signature, multi-tenant isolation)
4. **Что ещё не готово** (live KYC, real LLM, УКЭП — pilot-readiness gap)

## Структура

```
demo/
├── README.md                     # этот файл
├── scenarios/
│   ├── 01-llc-happy-path.sh      # ООО, документы валидные → auto-approve
│   ├── 02-ip-manual-review.sh    # ИП, документы partial → manual review
│   └── 03-ao-rejected.sh         # АО с sanctions match → declined
├── seed/
│   ├── tenant-demo-bank.json     # tenant payload
│   ├── llc-happy.json            # три заявки в JSON
│   ├── ip-partial.json
│   └── ao-sanctions.json
├── run-demo.sh                   # orchestrator всех 3 сценариев
└── teardown.sh                   # очистка state после demo
```

## Quick start

Перед запуском demo — поднять стек:

```bash
make up
make migrate-platform
make smoke              # проверить что health всех 21 сервиса OK
```

Затем:

```bash
cd demo
./run-demo.sh           # ~5 минут на все 3 сценария
```

Скрипт выводит цветной progress (зелёные галочки на успешных шагах,
красные кресты на ошибках), а в конце — summary-таблицу со всеми
заявками и их финальными решениями.

## Что показывать на встрече

См. [docs/demo-script.md](../docs/demo-script.md) — пошаговый walkthrough
с talking points (15-20 минут). И [docs/demo-slide-deck-outline.md](../docs/demo-slide-deck-outline.md)
— структура презентации (10-12 слайдов).

## Что в каждом сценарии

| Сценарий | ОПФ | Документы | Риск | Решение | Что демонстрирует |
|----------|-----|-----------|------|---------|-------------------|
| 01 LLC happy | ООО | паспорт + устав + ОГРН | low | auto-approve | full happy-path, AI-агенты, audit log |
| 02 IP manual | ИП | только паспорт (нет ОГРНИП) | medium | manual_review | escalation в operator UI, decision modal |
| 03 AO rejected | АО | устав + ОГРН | high (sanctions) | declined | adversarial flow, причина в audit, ext-rosfinmon hit |

## Для CISO банка

Дополнительные демонстрации:

```bash
# Audit chain integrity
./tools/audit-verifier verify --tenant-id=demo --source=db --dsn=$DATABASE_URL

# Tampering detection (запустить после demo)
psql -c "ALTER TABLE audit.events DISABLE TRIGGER no_update; \
         UPDATE audit.events SET payload='{\"tampered\":true}' WHERE id=...; \
         ALTER TABLE audit.events ENABLE TRIGGER no_update;"
./tools/audit-verifier verify --tenant-id=demo --source=db --dsn=$DATABASE_URL
# → Exit 1 + "TAMPERING ALERT (P0)"

# Multi-tenant isolation
psql -c "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tnt_%'"
# → tnt_demo (только этот тенант, изоляция через schema-per-tenant per ADR-0002)
```

## Что НЕ работает в demo

- **Реальные KYC API** (ФНС, Росфинмон, ФССП, СПАРК): ext-* сервисы
  возвращают синтетику (детерминированную по hash(INN)). Live-режим —
  при флипе `EGRUL_LIVE=true` после ФНС-договора через Минцифры.
- **Реальные LLM модели**: llm-gateway в mock-backend режиме. После
  получения GPU и развёртывания vLLM с Gemma 4 / Qwen 3.5 / T-pro —
  переключение через `VLLM_GEMMA_URL` ENV.
- **УКЭП клиента**: подписание документов клиентом — stub (показываем
  workflow, но без реальной криптоподписи). После КриптоПро/VipNet
  лицензий — реальная подпись через `packages/signature/cryptopro/`.

См. полный gap: [docs/pilot-readiness.md](../docs/pilot-readiness.md).

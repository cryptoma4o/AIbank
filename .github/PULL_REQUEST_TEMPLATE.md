<!-- Спасибо за вклад в AIbank. Заполни секции — это сэкономит время ревьюеру. -->

## Summary

<!-- 1-3 предложения: что и зачем. Ссылка на issue/Jira. -->

-

## Linked ADR

<!-- Если PR реализует или меняет архитектурное решение, укажи ADR. -->

- ADR-NNNN: <!-- e.g. docs/adr/0005-db-migrations-strategy.md -->
- N/A — изменение не требует ADR

## Test plan

<!-- Bulleted checklist. Минимум: какие команды запускались, какие проходят. -->

- [ ] `make test-go` — проходит локально
- [ ] `make test-python` — проходит локально (если затронут ai/ или packages/*/python)
- [ ] `npm run build` в затронутых apps/ — проходит
- [ ] Покрытие тестами для нового кода ≥ 70% (если применимо)
- [ ] `make smoke` — проходит против локального стека (для PR в main)

## Security checklist

<!-- Заполни обязательно для любого PR, который затрагивает обработку данных. -->

- [ ] Не добавлено логирование/хранение ПДн в plain-text (паспорт, телефон, СНИЛС, ФИО, ИНН ФЛ)
- [ ] Все новые поля с ПДн помечены `pii: true` в domain-model schema
- [ ] Не закоммичены секреты (API keys, JWT signing keys, DB passwords, .env)
- [ ] Новые внешние HTTP/gRPC вызовы используют timeout + circuit breaker
- [ ] Аутентификация/авторизация: новые endpoints проверяют tenant scope
- [ ] Audit-события публикуются для всех state-changing операций
- [ ] Криптография: использованы только утверждённые алгоритмы (см. ADR-0007)

## Eval impact (для PR в `ai/`)

<!-- Заполни если меняется промпт, цепочка агентов, RAG-pipeline, scoring. -->

- [ ] N/A — PR не затрагивает поведение AI-агентов
- [ ] Прогнан eval-harness против затронутых корпусов: <!-- список -->
- [ ] Метрики не деградировали > 5% (приложи diff из eval report)
- [ ] Новые edge-кейсы добавлены в датасеты `ai/eval-harness/datasets/`

## Migration impact (см. ADR-0005)

<!-- Заполни если PR содержит изменения схемы БД. -->

- [ ] N/A — нет миграций
- [ ] Миграция файла: `services/<svc>/migrations/<scope>/NNN_*.up.sql` + `*.down.sql`
- [ ] Up-миграция идемпотентна (можно прогнать дважды)
- [ ] Down-миграция протестирована на pre-prod
- [ ] Если drop column / NOT NULL constraint — разделено на 2 релиза (см. ADR-0005 § Политика отката)
- [ ] Применено к **platform** или **tenant** scope (укажи): <!-- platform | tenant -->
- [ ] Backfill план описан (если требуется)

## Backwards compatibility

<!-- Совместимость с N-1 версией сервисов. -->

- [ ] API: нет breaking changes в публичных endpoint-ах (или зафиксировано в ADR)
- [ ] gRPC/proto: только additive изменения (новые поля, новые messages)
- [ ] OpenAPI: добавлен новый версионный путь (v1 -> v2) при breaking
- [ ] Совместимо с N-1 deployment в SaaS rolling-update сценарии
- [ ] On-prem-апгрейд (см. ADR по on-prem cycle): миграция bundle включает шаги отката

## Screenshots / artifacts

<!-- Скриншоты UI, графики метрик, ссылки на CI builds. -->

---
<sub>Автоматически проверится: lint (Go/Python/TS), build, codegen-drift, helm-lint, openapi-lint. Один зелёный билд — обязателен.</sub>

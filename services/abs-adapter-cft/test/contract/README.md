# abs-adapter-cft — Contract Tests

Pre-integration golden-fixture тесты для адаптера ЦФТ. Закрывают Group A блокер
пилота: до подписания договора с банком на боевую интеграцию ЦФТ нам нужен
стабильный контракт между abs-connector ↔ adapter ↔ ЦФТ-XML.

См. ADR-0006 § 5 «Implementation Notes — golden-tests + mock-CFT (Phase 2)».

## Что покрыто

Четыре canonical-команды:

| Команда           | Canonical pair                                            | ЦФТ XML pair                                              |
|-------------------|-----------------------------------------------------------|-----------------------------------------------------------|
| OpenAccount (LLC) | `open_account_llc.request.json` / `*.expected.json`       | `open_account_llc.cft.request.xml` / `*.cft.response.xml` |
| GetAccountInfo    | `get_balance.request.json` / `*.expected.json`            | `get_balance.cft.request.xml` / `*.cft.response.xml`      |
| CloseAccount      | `close_account.request.json` / `*.expected.json`          | `close_account.cft.request.xml` / `*.cft.response.xml`    |
| Unknown command   | `reject_unknown_command.request.json` / `*.expected.json` | (нет — adapter не должен формировать XML на unknown)      |

Тесты проверяют две независимые проекции контракта:

1. **Canonical round-trip** — POST `/v1/execute` с canonical-request.json
   возвращает байт-в-байт canonical-expected.json (плюс детерминизм при
   повторном POST с тем же `idempotency_key`).
2. **Canonical ↔ ЦФТ XML translation** — `buildCFTRequest()` строит ЦФТ-XML,
   совпадающий с `*.cft.request.xml` (whitespace-insensitive); и обратно,
   `parseCFTResponse()` парсит pre-canned `*.cft.response.xml` в canonical
   `ABSResponse.Data`, совпадающую с `expected.json`.

## Запуск

```bash
# из services/abs-adapter-cft/
go test -tags=contract -v ./test/contract/...
```

Build-tag `contract` намеренно отделяет эти тесты от unit-тестов в
`internal/handler/` — последние гоняются на каждом push'е, contract-тесты
запускаются отдельной CI-стадой `make test-contract` (после фикса pipeline,
см. issue #TBD).

Проверить, что unit-тесты не сломаны:

```bash
go test ./internal/...
```

## Как добавить новую команду

1. Расширить `internal/domain/command.go` (новый `CommandType`) и
   `internal/handler/http.go` (case в `stub()`).
2. Создать четыре fixture-файла в `fixtures/`:
   - `<name>.request.json` — canonical ABSCommand
   - `<name>.expected.json` — canonical ABSResponse
   - `<name>.cft.request.xml` — ЦФТ-формат запроса
   - `<name>.cft.response.xml` — ЦФТ-формат ответа
3. Добавить запись в `canonicalCases` (для round-trip) и `cftTranslationCases`
   (для XML-translation) в `contract_test.go`.
4. Расширить `canonicalToCFTOperation()` и switch-блоки в `buildCFTRequest()` /
   `parseCFTResponse()`.
5. Прогнать `go test -tags=contract -v ./test/contract/...` и убедиться, что
   golden-значения совпадают. Если нет — НЕ редактировать fixture вслепую,
   разобраться, какой инвариант сломался (детерминизм SHA-256? новый ключ
   в payload? regression в handler?).

## Детерминизм

Все идентификаторы в `expected.json` (account_number, client_id, hex-suffix)
вычислены через `sha256(command + "|" + idempotency_key)` — алгоритм описан
в `internal/handler/http.go` (`digestHex`, `digestDigits`). Изменение алгоритма
требует **одновременного** обновления всех golden-фикстур и upgrade plan'а в
ADR (commit-сообщением «contract:bump-fixtures»).

В fixture **запрещены**:

- timestamp'ы (`time.Now()` и т.п.)
- random-значения
- runtime-зависимые поля (UUID, request_id, hostname)

## Translation helpers

`buildCFTRequest()` / `parseCFTResponse()` сейчас живут в самом тест-файле,
потому что production-handler в текущей итерации возвращает SHA-256-stub без
реального XML. Когда ADR-0006 § 5 Phase 2 закроется (реальный SOAP/MQ-клиент
ЦФТ), эти helper'ы переедут в `internal/handler/cft_translator.go`, а
contract-тесты будут импортировать их как нормальный пакет.

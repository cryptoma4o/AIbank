# mock-smev

Имитатор SMEV3-эндпоинта ФНС ЕГРЮЛ/ЕГРИП для pre-integration окружения AIbank.

Используется сервисом `services/ext-egrul` в режиме `EGRUL_LIVE=true` до момента
получения реального доступа к API ФНС (lead-time 4–8 недель после подписания
договора через Минцифры/СМЭВ-3).

## Что делает

- `GET /by-inn/{inn}` — возвращает XML карточки ЮЛ/ИП. XML детерминированно
  строится из хэша ИНН и совместим с `xml_mapper.MapXMLToLegalEntity` в
  ext-egrul (тот же набор полей, тот же формат `DD.MM.YYYY` для даты регистрации,
  тот же формат `<charter-capital-rub>` в рублях).
- `GET /by-ogrn/{ogrn}` — то же по ОГРН (ОГРН в ответе сохраняется как
  запрошенный).
- `GET /by-inn/9999999999` — намеренно `404`, для тестов «not found» в LiveProvider.
- `GET /healthz` — liveness JSON.

## Запуск

```bash
# локально
go run ./cmd/server          # :8500

# через docker
docker build -t aibank/mock-smev .
docker run --rm -p 8500:8500 aibank/mock-smev
```

## Конфигурация

| ENV  | Default | Описание                |
|------|---------|--------------------------|
| PORT | 8500    | TCP-порт HTTP-сервера    |

## Использование из ext-egrul

```bash
EGRUL_LIVE=true \
EGRUL_LIVE_ENDPOINT=http://localhost:8500 \
EGRUL_LIVE_TIMEOUT_MS=5000 \
go run ./cmd/server
```

После этого `GET /v1/egrul/by-inn/{inn}` в ext-egrul пойдёт в mock-smev,
получит XML, прогонит через `MapXMLToLegalEntity` и отдаст `LegalEntity` JSON.

## Тесты

```bash
go test ./...
```

Покрывает: happy-path по ИНН/ОГРН, 404 для зарезервированного `9999999999`,
детерминизм для одного ИНН, разный XML для разных ИНН, ИП-формат (12 цифр →
`egrip-record`), валидацию формата.

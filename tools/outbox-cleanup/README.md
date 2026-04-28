# outbox-cleanup

CLI-утилита, удаляющая published-строки старше retention из outbox-таблицы
любого сервиса, использующего [`packages/outbox`](../../packages/outbox/).

Назначение — запускаться как Kubernetes CronJob (см.
[`infrastructure/helm/charts/aibank-service/templates/cleanup-cronjob.yaml`](../../infrastructure/helm/charts/aibank-service/templates/cleanup-cronjob.yaml))
и контролировать рост outbox-таблицы. Закрывает TODO из
`packages/outbox/README.md` («Cleanup published-rows»).

## Зачем

Партиал-индекс по `published_at IS NULL` спасает relay от full-scan'а, но
сами published-строки в таблице остаются вечно. Для тенантов с десятками
тысяч событий в день это превращается в неконтролируемый рост дискового
пространства и медленный VACUUM.

`outbox-cleanup` — простой ответственный за этот мусор воркер: один SQL,
итеративно по batch'ам, безопасно для активного relay'я.

## Как работает

```
DELETE FROM <table> WHERE id IN (
  SELECT id FROM <table>
  WHERE published_at IS NOT NULL
    AND published_at < NOW() - <retention>
  ORDER BY id
  LIMIT <batch_size>
  FOR UPDATE SKIP LOCKED
)
```

* `published_at IS NOT NULL` — трогаем только то, что relay уже
  отправил в Kafka. Unpublished-строки не страдают.
* `SKIP LOCKED` — параллельный relay (или второй cleanup-запуск)
  не блокируется, мы пропускаем залоченные строки и берём следующие.
* `LIMIT batch_size` + цикл — на больших таблицах не держим один
  гигантский DELETE-tx; не раздуваем WAL.

См. реализацию: [`packages/outbox/cleanup.go`](../../packages/outbox/cleanup.go).

## Конфигурация (env-only)

| Переменная              | Обязательна | Дефолт | Описание                                                              |
|-------------------------|-------------|--------|------------------------------------------------------------------------|
| `DATABASE_URL`          | да          | —      | Postgres DSN. Пример: `postgres://app:***@pg:5432/aibank?sslmode=require` |
| `OUTBOX_TABLE`          | да          | —      | Полное имя outbox-таблицы. Пример: `platform.billing_outbox`           |
| `OUTBOX_RETENTION_DAYS` | нет         | `7`    | Сколько дней держать published-строки.                                 |
| `OUTBOX_BATCH_SIZE`     | нет         | `1000` | Размер пачки на одну итерацию DELETE.                                  |

## Запуск локально

```bash
# Из корня монорепо
cd tools/outbox-cleanup
DATABASE_URL=postgres://aibank:aibank@localhost:5432/aibank?sslmode=disable \
OUTBOX_TABLE=platform.billing_outbox \
OUTBOX_RETENTION_DAYS=7 \
go run ./cmd/outbox-cleanup
```

## Сборка Docker-образа

Build context должен быть корнем монорепо, потому что `go.mod`
использует `replace` на `../../packages/outbox` и Dockerfile копирует оба
дерева.

```bash
# Из корня монорепо
docker build \
  -f tools/outbox-cleanup/Dockerfile \
  -t registry.aibank.local/outbox-cleanup:0.1.0 \
  .
```

## Коды выхода

| Код | Значение                                                          |
|-----|-------------------------------------------------------------------|
| 0   | Cleanup отработал (включая 0 удалённых — это норма).              |
| 1   | Ошибка конфигурации (нет DATABASE_URL / OUTBOX_TABLE) или БД-сбой.|

## Безопасность

* **Не трогает unpublished**. Юнит-тест
  `TestCleanupPublished_DoesNotTouchUnpublished` падает при регрессии.
* **Не блокирует relay**. `SKIP LOCKED` гарантирует, что cleanup и relay
  никогда не дерутся за одну строку (и логически не пересекаются — relay
  работает только с unpublished, cleanup только с published).
* **Принимает SIGTERM**. На k8s eviction между итерациями выходит
  чисто, текущий DELETE доезжает до конца своего LIMIT'а.
* **Идемпотентен**. Повторный запуск после успешного — 0 удалений, ноль
  побочных эффектов.

## Связанные файлы

- [`packages/outbox/cleanup.go`](../../packages/outbox/cleanup.go) — реализация `CleanupPublished`.
- [`packages/outbox/cleanup_test.go`](../../packages/outbox/cleanup_test.go) — unit-тесты на sqlmock.
- [`infrastructure/helm/charts/aibank-service/templates/cleanup-cronjob.yaml`](../../infrastructure/helm/charts/aibank-service/templates/cleanup-cronjob.yaml) — Helm-шаблон CronJob.
- [`packages/outbox/README.md`](../../packages/outbox/README.md) — документация outbox-пакета.

# packages/outbox

Reusable Go-пакет реализующий **transactional outbox pattern** (ADR-0010 § 2)
для at-least-once-доставки доменных событий в Kafka.

## Зачем

Прямая публикация в Kafka из сервиса теряет события при сбое между «БД commit»
и «Kafka publish». Outbox + background relay — стандартное решение этой
проблемы:

1. **Producer side** — в одной DB-транзакции с доменным INSERT'ом пишется
   строка в outbox-таблицу. Если транзакция откатилась, outbox-запись тоже
   откатилась.
2. **Relay** — отдельная горутина SELECT'ит unpublished-строки, публикует в
   Kafka, помечает `published_at = NOW()`.
3. **Concurrent safety** — `SELECT ... FOR UPDATE SKIP LOCKED` гарантирует,
   что несколько запущенных одновременно relay-инстансов не дублируют работу.
4. **Idempotency** — consumer-side дедуп (см. ADR-0010 § 5: `ON CONFLICT (id)
   DO NOTHING`). At-least-once + idempotent consumer = effectively-exactly-once.

## Использование

### Шаг 1. Миграция

Скопируйте `migrations/001_create_outbox.sql.template` в свой сервис, замените
`{{SCHEMA}}` и `{{TABLE}}` на нужные значения (например, `platform` /
`billing_outbox`).

### Шаг 2. EnqueueTx в доменной транзакции

```go
import "github.com/aibank/platform/packages/outbox"

ob, _ := outbox.New(db, "platform.billing_outbox", "platform.billing.events")

tx, _ := db.BeginTx(ctx, nil)
defer tx.Rollback()

// Доменный INSERT
tx.ExecContext(ctx, "INSERT INTO platform.billing_events ...")

// Outbox INSERT в той же tx — атомарно с доменным
payload, _ := json.Marshal(billingEvent)
ob.EnqueueTx(ctx, tx, "billing_event", billingEvent.ID, billingEvent.EventType, payload)

tx.Commit()
```

### Шаг 3. Запуск relay

```go
pub := outbox.NewKafkaPublisher([]string{"kafka-1:9092", "kafka-2:9092"},
    "platform.billing.events")
defer pub.Close()

relay, _ := outbox.NewRelay(ob, pub, outbox.RelayOptions{
    PollInterval: 5 * time.Second,
    BatchSize:    100,
    Logger:       logger,
})

go relay.Run(ctx) // блокируется до ctx.Done()
```

### Шаг 4. Тестирование без Kafka

```go
stub := &outbox.StubPublisher{}
relay, _ := outbox.NewRelay(ob, stub, outbox.RelayOptions{})

relay.Tick(ctx) // один цикл вместо Run

// stub.Messages теперь содержит все опубликованные сообщения
```

## Семантика отказов

* **Publish провалился** — строка остаётся unpublished, следующий tick
  повторит. Логируется как ERROR.
* **БД-сбой при SELECT/UPDATE** — Tick возвращает ошибку, Run логирует и
  продолжает на следующем тике (не падает).
* **ctx.Done()** — Run выходит штатно, текущая транзакция откатывается;
  unpublished-строки подберёт следующий запуск.

## TODO

* **DLQ для poison messages** — события, которые валятся на Publish N раз
  подряд, перекладывать в `*_dead_letter`. Сейчас они просто бесконечно
  ретраятся.
* **Cleanup published-rows** — partial-index спасает relay, но таблица всё
  равно растёт. Добавить cron-cleanup `DELETE WHERE published_at < NOW() -
  INTERVAL '7 days'` (но не раньше, чем consumer-side дедуп даст гарантию).
* **Метрики** — `outbox_relay_published_total`, `outbox_relay_lag_seconds`,
  `outbox_relay_publish_errors_total` для VictoriaMetrics.

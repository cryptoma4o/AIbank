// Package outbox реализует transactional outbox pattern (ADR-0010 § 2):
// доменные изменения и outbox-запись пишутся в одной БД-транзакции, а отдельный
// background relay читает unpublished-строки из таблицы и публикует их в Kafka,
// после чего помечает строку как published_at = NOW().
//
// Это гарантирует at-least-once-доставку события в Kafka даже при крэше сервиса
// между БД-commit и публикацией: пропавшая публикация подхватится следующим
// поллом relay.
package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"
)

// Publisher — абстракция над Kafka producer'ом. Тесты используют StubPublisher,
// production — KafkaPublisher.
type Publisher interface {
	Publish(ctx context.Context, key, value []byte) error
	Close() error
}

// Outbox — настройки и БД-handle для outbox-таблицы. Не содержит состояния:
// безопасно использовать из нескольких goroutine.
type Outbox struct {
	db    *sql.DB
	table string // полное имя таблицы, например "platform.billing_outbox"
	topic string // Kafka topic, например "platform.billing.events"
}

// New конструирует Outbox. table должен включать схему ("platform.billing_outbox"),
// topic — целевой Kafka topic.
func New(db *sql.DB, table, topic string) (*Outbox, error) {
	if db == nil {
		return nil, errors.New("outbox: db is required")
	}
	if table == "" {
		return nil, errors.New("outbox: table is required")
	}
	if topic == "" {
		return nil, errors.New("outbox: topic is required")
	}
	return &Outbox{db: db, table: table, topic: topic}, nil
}

// Table возвращает полное имя таблицы (для диагностики/тестов).
func (o *Outbox) Table() string { return o.table }

// Topic возвращает Kafka topic.
func (o *Outbox) Topic() string { return o.topic }

// EnqueueTx вставляет outbox-запись в указанной транзакции. Должно вызываться
// внутри той же tx, что доменный INSERT — это и есть суть outbox pattern.
//
// aggregateType — например "billing_event"; aggregateID — id агрегата
// (используется в качестве key в Kafka для партиционирования);
// eventType — каноничный тип события (см. domain.EventType*);
// payload — JSON-готовый bytes (обычно сериализованный domain.BillingEvent).
func (o *Outbox) EnqueueTx(
	ctx context.Context,
	tx *sql.Tx,
	aggregateType, aggregateID, eventType string,
	payload []byte,
) error {
	if tx == nil {
		return errors.New("outbox: tx is required")
	}
	if aggregateType == "" || aggregateID == "" || eventType == "" {
		return errors.New("outbox: aggregate_type/aggregate_id/event_type required")
	}
	if len(payload) == 0 {
		return errors.New("outbox: payload required")
	}
	q := fmt.Sprintf(
		`INSERT INTO %s (aggregate_type, aggregate_id, event_type, payload)
		 VALUES ($1, $2, $3, $4)`, o.table)
	if _, err := tx.ExecContext(ctx, q, aggregateType, aggregateID, eventType, payload); err != nil {
		return fmt.Errorf("outbox: enqueue: %w", err)
	}
	return nil
}

// KafkaPublisher — Publisher на базе segmentio/kafka-go Writer.
type KafkaPublisher struct {
	w *kafkago.Writer
}

// NewKafkaPublisher строит Writer с RequiredAcks=All для надёжной доставки.
// brokers — CSV или slice broker-host:port'ов.
func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: false,
		Compression:            kafkago.Lz4,
	}
	return &KafkaPublisher{w: w}
}

// Publish отправляет одно сообщение синхронно (Writer всё равно блокируется
// до подтверждения от брокера при RequiredAcks=All).
func (p *KafkaPublisher) Publish(ctx context.Context, key, value []byte) error {
	return p.w.WriteMessages(ctx, kafkago.Message{Key: key, Value: value})
}

// Close закрывает Writer, дожидаясь сброса всех буферов.
func (p *KafkaPublisher) Close() error {
	if p.w == nil {
		return nil
	}
	return p.w.Close()
}

// StubPublisher — in-memory Publisher для тестов. Записывает все Publish-вызовы
// в Messages. Не потокобезопасный — тесты не должны вызывать concurrent Publish.
type StubPublisher struct {
	Messages []StubMessage
	// Err, если не nil, возвращается всеми Publish-вызовами (для теста error-path).
	Err error
}

// StubMessage — сохранённый Publish.
type StubMessage struct {
	Key   []byte
	Value []byte
}

// Publish реализует Publisher: сохраняет (key, value) в Messages.
func (p *StubPublisher) Publish(_ context.Context, key, value []byte) error {
	if p.Err != nil {
		return p.Err
	}
	// Копируем bytes, потому что вызывающий код может реюзать буфер.
	k := append([]byte(nil), key...)
	v := append([]byte(nil), value...)
	p.Messages = append(p.Messages, StubMessage{Key: k, Value: v})
	return nil
}

// Close для StubPublisher — noop.
func (p *StubPublisher) Close() error { return nil }

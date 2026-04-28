package domain

import (
	"context"
	"errors"
	"time"
)

// ErrIdempotencyMiss — служебная ошибка, в публичный API не возвращается;
// hit/miss выражается через `(value, found, error)` контракт ниже.
// Объявляем её, чтобы реализация (Redis/InMemory) могла различать
// «ключа нет» vs «ошибка транспорта», но handler'у достаточно `found bool`.
var ErrIdempotencyMiss = errors.New("idempotency miss")

// IdempotencyStore — единственный канал «знаем ли мы уже ответ
// на этот idempotency_key». Согласно ADR-0006 dedup-семантика — часть
// канонического контракта: смена ключа = breaking change адаптера,
// а кеширование ответа на N часов — обязательная гарантия connector'а
// для безопасных retry-сценариев в Temporal SAGA (ADR-0001).
//
// Контракт реализаций:
//   - Get возвращает (response, true, nil) при попадании;
//     (CanonicalResponse{}, false, nil) при чистом miss;
//     (CanonicalResponse{}, false, err) при transport-ошибке.
//   - Put обязан атомарно сохранить response на ttl. Если ttl == 0 —
//     реализация вправе использовать свой default (24h в Redis,
//     бесконечно в InMemory).
//   - Реализации НЕ обязаны защищаться от concurrent Put для одного key:
//     дубль-исполнение защищается на уровне adapter'а (его собственный
//     dedup-store) либо на уровне Temporal-WF'а; connector'овский cache —
//     это performance/UX-оптимизация для повторов, не консистентный lock.
type IdempotencyStore interface {
	Get(ctx context.Context, tenantID, idempotencyKey string) (CanonicalResponse, bool, error)
	Put(ctx context.Context, tenantID, idempotencyKey string, response CanonicalResponse, ttl time.Duration) error
}

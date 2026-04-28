// Package provider определяет абстракцию поставщика данных ЕГРЮЛ/ЕГРИП.
//
// Провайдер инкапсулирует источник истины: синтетический генератор (для dev/CI/preprod)
// или реальный API ФНС (live). Хэндлер не знает, какой импл подключён,
// и переключение делается через ENV-флаг + factory.
//
// См.:
//
//	synthetic.go  — обёртка над domain.Synthetic*
//	live.go       — заглушка реального клиента ФНС
//	factory.go    — выбор по ENV (EGRUL_LIVE)
package provider

import (
	"context"
	"errors"

	"aibank/ext-egrul/internal/domain"
)

// ErrNotImplemented — стандартный sentinel для LiveProvider до реальной интеграции.
//
// Хэндлер должен пробрасывать эту ошибку как 503 Service Unavailable, а
// эксплуатация — мониторить её отсутствие на проде после флипа EGRUL_LIVE=true.
var ErrNotImplemented = errors.New("provider: метод не реализован — необходима реальная интеграция с ФНС")

// Provider — абстракция источника данных ЕГРЮЛ.
//
// Контракт совпадает с тем, что использует HTTP-обработчик: синтетический и
// реальный поставщики взаимозаменяемы.
type Provider interface {
	// GetByINN возвращает карточку ЮЛ/ИП по ИНН.
	GetByINN(ctx context.Context, inn string) (domain.LegalEntity, error)

	// GetByOGRN возвращает карточку по ОГРН/ОГРНИП.
	GetByOGRN(ctx context.Context, ogrn string) (domain.LegalEntity, error)

	// GetFounders возвращает учредителей ЮЛ. Для ИП — пустой список.
	GetFounders(ctx context.Context, inn string) ([]domain.Founder, error)

	// Name — короткое имя реализации для логов и метрик ("synthetic" | "live").
	Name() string
}

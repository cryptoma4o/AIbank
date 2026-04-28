// Package provider определяет абстракцию поставщика корпоративной аналитики
// (СПАРК / Контур.Фокус).
//
// Провайдер инкапсулирует источник истины: синтетический генератор (для dev/CI/preprod)
// или реальный API провайдера (live). Хэндлер не знает, какой импл подключён,
// и переключение делается через ENV-флаг + factory.
package provider

import (
	"context"
	"errors"

	"aibank/ext-spark/internal/domain"
)

// ErrNotImplemented — стандартный sentinel для LiveProvider до реальной интеграции.
var ErrNotImplemented = errors.New("provider: метод не реализован — необходима реальная интеграция со СПАРК/Контур.Фокус")

// Provider — абстракция источника корпоративной аналитики.
type Provider interface {
	// GetIntel возвращает корпоративные сигналы по ИНН.
	GetIntel(ctx context.Context, inn string) (domain.SparkIntel, error)

	// Name — короткое имя реализации.
	Name() string
}

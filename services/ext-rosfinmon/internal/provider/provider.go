// Package provider определяет абстракцию поставщика данных Росфинмониторинга
// (перечень 115-ФЗ).
//
// Провайдер инкапсулирует источник истины: синтетический генератор (для dev/CI/preprod)
// или реальный feed Росфинмониторинга (live). Хэндлер не знает, какой импл подключён,
// и переключение делается через ENV-флаг + factory.
package provider

import (
	"context"
	"errors"

	"aibank/ext-rosfinmon/internal/domain"
)

// ErrNotImplemented — стандартный sentinel для LiveProvider до реальной интеграции.
var ErrNotImplemented = errors.New("provider: метод не реализован — необходима реальная интеграция с Росфинмониторингом")

// Provider — абстракция источника перечня 115-ФЗ.
type Provider interface {
	// Screen проверяет субъект по перечню.
	Screen(ctx context.Context, req domain.ScreeningRequest) (domain.ScreeningResult, error)

	// GetSnapshot возвращает метаданные (last_updated, count) текущего перечня.
	GetSnapshot(ctx context.Context) (domain.ListSnapshot, error)

	// Name — короткое имя реализации для логов и метрик ("synthetic" | "live").
	Name() string
}

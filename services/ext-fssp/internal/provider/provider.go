// Package provider определяет абстракцию поставщика данных ФССП
// (исполнительные производства).
//
// Провайдер инкапсулирует источник истины: синтетический генератор (для dev/CI/preprod)
// или реальный API ФССП (live). Хэндлер не знает, какой импл подключён,
// и переключение делается через ENV-флаг + factory.
package provider

import (
	"context"
	"errors"

	"aibank/ext-fssp/internal/domain"
)

// ErrNotImplemented — стандартный sentinel для LiveProvider до реальной интеграции.
var ErrNotImplemented = errors.New("provider: метод не реализован — необходима реальная интеграция с ФССП")

// Provider — абстракция источника исполнительных производств.
type Provider interface {
	// GetByINN возвращает производства по ИНН (ЮЛ/ИП).
	GetByINN(ctx context.Context, inn string) (domain.ProceedingsResult, error)

	// GetByPerson возвращает производства по ФЛ (ФИО + дата рождения).
	GetByPerson(ctx context.Context, fullName, birthDate string) (domain.ProceedingsResult, error)

	// Name — короткое имя реализации.
	Name() string
}

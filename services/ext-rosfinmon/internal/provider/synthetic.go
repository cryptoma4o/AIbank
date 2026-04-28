package provider

import (
	"context"

	"aibank/ext-rosfinmon/internal/domain"
)

// SyntheticProvider — обёртка над domain.Synthetic* функциями.
//
// Используется по умолчанию (когда RFM_LIVE != "true"). Перечень детерминирован,
// матчинг даёт ~1% «фантомных» совпадений, чтобы тестировался false-positive flow.
type SyntheticProvider struct{}

// NewSyntheticProvider возвращает provider, дающий синтетические ответы.
func NewSyntheticProvider() *SyntheticProvider {
	return &SyntheticProvider{}
}

// Name — имя реализации.
func (p *SyntheticProvider) Name() string { return "synthetic" }

// Screen — синтетическая проверка по перечню.
func (p *SyntheticProvider) Screen(_ context.Context, req domain.ScreeningRequest) (domain.ScreeningResult, error) {
	return domain.Screen(req), nil
}

// GetSnapshot возвращает синтетические метаданные перечня.
func (p *SyntheticProvider) GetSnapshot(_ context.Context) (domain.ListSnapshot, error) {
	return domain.CurrentSnapshot(), nil
}

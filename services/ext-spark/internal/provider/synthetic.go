package provider

import (
	"context"

	"aibank/ext-spark/internal/domain"
)

// SyntheticProvider — обёртка над domain.SyntheticIntel.
//
// Используется по умолчанию (когда SPARK_LIVE != "true"). Распределение
// financial_health детерминированное: ~60% green, ~30% yellow, ~10% red.
type SyntheticProvider struct{}

// NewSyntheticProvider возвращает provider, дающий синтетические ответы.
func NewSyntheticProvider() *SyntheticProvider {
	return &SyntheticProvider{}
}

// Name — имя реализации.
func (p *SyntheticProvider) Name() string { return "synthetic" }

// GetIntel — синтетика по ИНН.
func (p *SyntheticProvider) GetIntel(_ context.Context, inn string) (domain.SparkIntel, error) {
	return domain.SyntheticIntel(inn), nil
}

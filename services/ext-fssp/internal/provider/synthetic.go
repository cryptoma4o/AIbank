package provider

import (
	"context"

	"aibank/ext-fssp/internal/domain"
)

// SyntheticProvider — обёртка над domain.Synthetic* функциями.
//
// Используется по умолчанию (когда FSSP_LIVE != "true"). ~5% запросов имеют ≥1
// производство — детерминировано от хэша ИНН/(ФИО+дата).
type SyntheticProvider struct{}

// NewSyntheticProvider возвращает provider, дающий синтетические ответы.
func NewSyntheticProvider() *SyntheticProvider {
	return &SyntheticProvider{}
}

// Name — имя реализации.
func (p *SyntheticProvider) Name() string { return "synthetic" }

// GetByINN — синтетика по ИНН.
func (p *SyntheticProvider) GetByINN(_ context.Context, inn string) (domain.ProceedingsResult, error) {
	return domain.SyntheticByINN(inn), nil
}

// GetByPerson — синтетика по ФЛ.
func (p *SyntheticProvider) GetByPerson(_ context.Context, fullName, birthDate string) (domain.ProceedingsResult, error) {
	return domain.SyntheticByPerson(fullName, birthDate), nil
}

package provider

import (
	"context"

	"aibank/ext-egrul/internal/domain"
)

// SyntheticProvider — обёртка над domain.Synthetic* функциями.
//
// Используется по умолчанию (когда EGRUL_LIVE != "true"). Все ответы
// детерминированно строятся из ИНН/ОГРН — пригодно для интеграционных тестов
// и demo-стендов без реального доступа к ФНС.
type SyntheticProvider struct{}

// NewSyntheticProvider возвращает provider, дающий синтетические ответы.
func NewSyntheticProvider() *SyntheticProvider {
	return &SyntheticProvider{}
}

// Name — имя реализации для логирования.
func (p *SyntheticProvider) Name() string { return "synthetic" }

// GetByINN строит синтетическую карточку ЮЛ/ИП.
func (p *SyntheticProvider) GetByINN(_ context.Context, inn string) (domain.LegalEntity, error) {
	return domain.SyntheticLegalEntity(inn), nil
}

// GetByOGRN строит синтетическую карточку по ОГРН.
func (p *SyntheticProvider) GetByOGRN(_ context.Context, ogrn string) (domain.LegalEntity, error) {
	return domain.SyntheticByOGRN(ogrn), nil
}

// GetFounders возвращает синтетический список учредителей.
func (p *SyntheticProvider) GetFounders(_ context.Context, inn string) ([]domain.Founder, error) {
	founders := domain.SyntheticFounders(inn)
	if founders == nil {
		founders = []domain.Founder{}
	}
	return founders, nil
}

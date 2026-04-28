package domain

import "context"

// UBOGraphRepository — операции над графами владения.
//
// Семантика «обновления» отсутствует: новый snapshot — это новая запись с
// version+1. ListByLegalEntity возвращает историю версий (от свежей к
// старой), GetLatestByLegalEntity — текущий граф для оркестратора/UI.
type UBOGraphRepository interface {
	Create(ctx context.Context, g *UBOGraph) error
	GetByID(ctx context.Context, tenantID, id string) (*UBOGraph, error)
	GetLatestByLegalEntity(ctx context.Context, tenantID, legalEntityID string) (*UBOGraph, error)
	ListByLegalEntity(ctx context.Context, tenantID, legalEntityID string, limit, offset int) ([]*UBOGraph, error)
}

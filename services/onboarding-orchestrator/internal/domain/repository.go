package domain

import "context"

// ApplicationRepository — порт для персистентного хранилища заявок.
//
// Реализация (см. internal/repository/postgres.go) обязана работать в
// контексте конкретной схемы тенанта (search_path), см. ADR-0002.
// tenantID передаётся явно — за выбор схемы отвечает реализация.
type ApplicationRepository interface {
	// Create вставляет новую заявку в схеме tnt_<tenantID>.
	// Поле State должно быть валидно (StateDraft или StateIdentifying на старте).
	Create(ctx context.Context, app *Application) error

	// GetByID возвращает заявку по ID в указанном тенанте.
	// Возвращает ErrNotFound, если запись отсутствует.
	GetByID(ctx context.Context, tenantID, id string) (*Application, error)

	// UpdateState переводит заявку в новое состояние.
	// Реализация ОБЯЗАНА проверить CanTransition(currentState, newState) и
	// вернуть ErrInvalidTransition, если переход запрещён.
	UpdateState(ctx context.Context, tenantID, id string, newState ApplicationState) error

	// ListByTenant возвращает заявки тенанта (упорядочены по created_at desc).
	ListByTenant(ctx context.Context, tenantID string, limit int) ([]*Application, error)
}

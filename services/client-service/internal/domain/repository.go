package domain

import "context"

// ClientRepository — операции над карточкой клиента.
//
// Update сущности целиком намеренно отсутствует: единственный доменно
// разрешённый mutate-сценарий — смена статуса (PATCH /status). Любые
// исторические изменения фиксируются как ClientHistory события, не
// перезаписью полей.
type ClientRepository interface {
	Create(ctx context.Context, c *Client) error
	GetByID(ctx context.Context, tenantID, id string) (*Client, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*Client, error)
	UpdateStatus(ctx context.Context, tenantID, id string, status ClientStatus) error
}

// ClientHistoryRepository — append-only журнал по клиенту.
//
// tenantID передаётся явно (не через поле сущности), потому что физически
// строка хранится в схеме tnt_<id> без колонки tenant_id (изоляция —
// schema-per-tenant, ADR-0002), и репозиторий должен знать схему для
// SET search_path.
type ClientHistoryRepository interface {
	Append(ctx context.Context, h *ClientHistory, tenantID string) error
	ListByClient(ctx context.Context, tenantID, clientID string, limit, offset int) ([]*ClientHistory, error)
}

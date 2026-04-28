package domain

import (
	"context"
	"encoding/json"
)

// DocumentRepository — порт доступа к метаданным документов в схеме тенанта.
// Все методы выполняются в контексте уже выставленного search_path.
type DocumentRepository interface {
	Create(ctx context.Context, doc *Document) error
	GetByID(ctx context.Context, tenantID, id string) (*Document, error)
	ListByApplication(ctx context.Context, tenantID, applicationID string) ([]*Document, error)
	MarkParsed(ctx context.Context, tenantID, id string, parsed json.RawMessage) error
	MarkRejected(ctx context.Context, tenantID, id, reason string) error
}

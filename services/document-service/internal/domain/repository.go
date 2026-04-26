package domain

import "context"

type DocumentRepository interface {
	GetByID(ctx context.Context, id string) (*Document, error)
	ListByApplication(ctx context.Context, applicationID string) ([]*Document, error)
	Create(ctx context.Context, doc *Document) error
	UpdateStatus(ctx context.Context, id string, status DocumentStatus) error
}

type StorageClient interface {
	// Upload stores document bytes and returns the storage path.
	Upload(ctx context.Context, tenantID, appID, docID, mimeType string, data []byte) (storagePath string, err error)
	// Download retrieves document bytes by storage path.
	Download(ctx context.Context, storagePath string) ([]byte, error)
	// Delete removes a document from storage.
	Delete(ctx context.Context, storagePath string) error
}

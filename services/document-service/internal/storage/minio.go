package storage

import (
	"context"
	"fmt"
)

// MinIOClient implements domain.StorageClient using MinIO S3-compatible API.
// Bucket naming: platform-{tenant-id}-documents
// Object path: {application_id}/{document_id}
type MinIOClient struct {
	endpoint  string
	accessKey string
	secretKey string
}

func NewMinIOClient(endpoint, accessKey, secretKey string) *MinIOClient {
	return &MinIOClient{endpoint: endpoint, accessKey: accessKey, secretKey: secretKey}
}

func (c *MinIOClient) bucketName(tenantID string) string {
	return fmt.Sprintf("platform-%s-documents", tenantID)
}

func (c *MinIOClient) objectKey(appID, docID string) string {
	return fmt.Sprintf("%s/%s", appID, docID)
}

func (c *MinIOClient) Upload(ctx context.Context, tenantID, appID, docID, mimeType string, data []byte) (string, error) {
	bucket := c.bucketName(tenantID)
	key := c.objectKey(appID, docID)
	// TODO: use minio-go client to put object
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil
}

func (c *MinIOClient) Download(ctx context.Context, storagePath string) ([]byte, error) {
	// TODO: parse storagePath and fetch from MinIO
	return nil, fmt.Errorf("not implemented")
}

func (c *MinIOClient) Delete(ctx context.Context, storagePath string) error {
	// TODO: parse storagePath and delete from MinIO
	return fmt.Errorf("not implemented")
}

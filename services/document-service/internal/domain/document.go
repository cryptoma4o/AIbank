package domain

import "time"

type DocumentType string

const (
	DocumentTypePassport     DocumentType = "passport"
	DocumentTypeCharter      DocumentType = "charter"
	DocumentTypeProtocol     DocumentType = "protocol"
	DocumentTypeEGRULExtract DocumentType = "egrul_extract"
	DocumentTypeINN          DocumentType = "inn_certificate"
	DocumentTypeOGRN         DocumentType = "ogrn_certificate"
	DocumentTypeBankCard     DocumentType = "bank_card"
	DocumentTypeOther        DocumentType = "other"
)

type DocumentStatus string

const (
	DocumentStatusUploaded  DocumentStatus = "uploaded"
	DocumentStatusParsing   DocumentStatus = "parsing"
	DocumentStatusParsed    DocumentStatus = "parsed"
	DocumentStatusValidated DocumentStatus = "validated"
	DocumentStatusRejected  DocumentStatus = "rejected"
)

type Document struct {
	ID            string         `json:"id"`
	ApplicationID string         `json:"application_id"`
	TenantID      string         `json:"tenant_id"`
	DocumentType  DocumentType   `json:"document_type"`
	Status        DocumentStatus `json:"status"`
	StoragePath   string         `json:"storage_path"`   // s3://platform-{tenant}-documents/{app_id}/{doc_id}
	FileSizeBytes int64          `json:"file_size_bytes"`
	MimeType      string         `json:"mime_type"`
	Checksum      string         `json:"checksum"`       // SHA-256 of file content
	UploadedAt    time.Time      `json:"uploaded_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

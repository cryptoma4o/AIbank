package domain

import (
	"encoding/json"
	"time"
)

// DocumentType — классификация документа.
// Соответствует docs/domain-model.md § 2.6 (упрощённый набор для MVP).
type DocumentType string

const (
	DocumentTypePassport        DocumentType = "passport"
	DocumentTypePassportForeign DocumentType = "passport_foreign"
	DocumentTypeCharter         DocumentType = "charter"
	DocumentTypeProtocol        DocumentType = "protocol"
	DocumentTypeExtract         DocumentType = "extract"
	DocumentTypeAgreement       DocumentType = "agreement"
)

// IsValidDocumentType проверяет, входит ли значение в whitelist типов.
func IsValidDocumentType(t DocumentType) bool {
	switch t {
	case DocumentTypePassport, DocumentTypePassportForeign,
		DocumentTypeCharter, DocumentTypeProtocol,
		DocumentTypeExtract, DocumentTypeAgreement:
		return true
	}
	return false
}

// DocumentState — конечный автомат состояний документа.
type DocumentState string

const (
	DocumentStateUploaded  DocumentState = "uploaded"
	DocumentStateParsed    DocumentState = "parsed"
	DocumentStateValidated DocumentState = "validated"
	DocumentStateRejected  DocumentState = "rejected"
)

// SourceType — откуда документ попал в систему.
type SourceType string

const (
	SourceClientUpload SourceType = "client_upload"
	SourceEGRUL        SourceType = "egrul"
	SourceESIA         SourceType = "esia"
	SourceGenerated    SourceType = "generated"
)

// IsValidSourceType проверяет валидность источника.
func IsValidSourceType(s SourceType) bool {
	switch s {
	case SourceClientUpload, SourceEGRUL, SourceESIA, SourceGenerated:
		return true
	}
	return false
}

// Document — единица хранения документа в схеме тенанта.
//
// Файл-контент находится в объектном хранилище (см. internal/storage), здесь —
// только метаданные и ссылка StoragePath. Поле ParsedData — JSON, заполняется
// document-intake-agent после OCR/извлечения; в этом сервисе не интерпретируется.
type Document struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	ApplicationID string          `json:"application_id"`
	Type          DocumentType    `json:"type"`
	State         DocumentState   `json:"state"`
	FileID        string          `json:"file_id"`
	Filename      string          `json:"filename"`
	MimeType      string          `json:"mime_type"`
	SizeBytes     int64           `json:"size_bytes"`
	SHA256        string          `json:"sha256"`
	StoragePath   string          `json:"storage_path"`
	SourceType    SourceType      `json:"source_type"`
	SourceActorID string          `json:"source_actor_id,omitempty"`
	ParsedData    json.RawMessage `json:"parsed_data,omitempty"`
	UploadedAt    time.Time       `json:"uploaded_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
